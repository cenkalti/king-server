package server

import (
	"context"
	"errors"
	"sync"

	"github.com/coder/websocket"

	"github.com/cenkalti/king-server/match"
	"github.com/cenkalti/king-server/protocol"
)

// RemotePlayer is a bot connected over WebSocket. It implements
// match.Player and match.Reconnector. Several requests may be in flight at
// once; responses are matched by id.
type RemotePlayer struct {
	name string
	logf func(format string, args ...any)

	mu        sync.Mutex
	conn      *websocket.Conn // nil while disconnected
	dropped   chan struct{}   // closed when conn drops
	connected chan struct{}   // closed while connected
	pending   map[string]chan *protocol.Response
}

func newRemotePlayer(name string) *RemotePlayer {
	return &RemotePlayer{
		name:      name,
		connected: make(chan struct{}),
		pending:   map[string]chan *protocol.Response{},
	}
}

func (p *RemotePlayer) Name() string { return p.name }

// Connected reports whether the bot currently has a connection.
func (p *RemotePlayer) Connected() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.conn != nil
}

// attach makes conn the bot's connection. It fails if another connection
// is already attached.
func (p *RemotePlayer) attach(conn *websocket.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		return false
	}
	p.conn = conn
	p.dropped = make(chan struct{})
	close(p.connected)
	return true
}

func (p *RemotePlayer) detach(conn *websocket.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != conn {
		return
	}
	p.conn = nil
	close(p.dropped)
	p.connected = make(chan struct{})
}

// readLoop delivers responses until the connection fails.
func (p *RemotePlayer) readLoop(conn *websocket.Conn) {
	defer conn.CloseNow()
	defer p.detach(conn)
	for {
		_, b, err := conn.Read(context.Background())
		if err != nil {
			return
		}
		resp, err := decodeResponse(b)
		if err != nil {
			p.log("%s: ignoring message: %v", p.name, err)
			continue
		}
		p.mu.Lock()
		ch := p.pending[resp.ID]
		delete(p.pending, resp.ID)
		p.mu.Unlock()
		if ch == nil {
			p.log("%s: ignoring late or unknown response %q", p.name, resp.ID)
			continue
		}
		ch <- resp // buffered
	}
}

func (p *RemotePlayer) log(format string, args ...any) {
	if p.logf != nil {
		p.logf(format, args...)
	}
}

// Decide sends req and waits for the matching response. It returns an error
// wrapping match.ErrDisconnected if the bot is not connected or its
// connection drops while waiting, and ctx.Err() if ctx ends first.
func (p *RemotePlayer) Decide(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	ch := make(chan *protocol.Response, 1)
	p.mu.Lock()
	conn, dropped := p.conn, p.dropped
	if conn == nil {
		p.mu.Unlock()
		return nil, match.ErrDisconnected
	}
	p.pending[req.ID] = ch
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.pending, req.ID)
		p.mu.Unlock()
	}()

	// The write gets its own deadline: coder/websocket closes the connection
	// if a write's context expires, and a slow decision must not do that.
	if err := write(conn, req); err != nil {
		conn.CloseNow()
		return nil, match.ErrDisconnected
	}
	select {
	case resp := <-ch:
		return resp, nil
	case <-dropped:
		return nil, match.ErrDisconnected
	case <-ctx.Done():
		// A bot that stops answering may really be a dead network path the
		// TCP stack hasn't noticed yet. Ping it: if it doesn't answer, treat
		// it as a disconnect (pause) rather than a timeout (strike).
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			pctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				conn.CloseNow()
				return nil, match.ErrDisconnected
			}
		}
		return nil, ctx.Err()
	}
}

// WaitConnected blocks until the bot is connected or ctx is done.
func (p *RemotePlayer) WaitConnected(ctx context.Context) error {
	p.mu.Lock()
	ch := p.connected
	p.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SendMatchEnd sends the final standings if the bot is connected, then closes
// the connection normally.
func (p *RemotePlayer) SendMatchEnd(standings []protocol.Standing) {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()
	if conn == nil {
		return
	}
	write(conn, protocol.MatchEnd{Type: protocol.TypeMatchEnd, Standings: standings})
	conn.Close(websocket.StatusNormalClosure, "match over")
}
