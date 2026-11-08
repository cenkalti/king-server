// Package server accepts bot connections over WebSocket and exposes each
// bot as a match.Player.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/cenkalti/king-server/protocol"
)

const (
	helloTimeout = 10 * time.Second
	writeTimeout = 10 * time.Second
	pingInterval = 15 * time.Second
	pingTimeout  = 3 * time.Second
	maxNameLen   = 32
)

// Hub accepts bots at its HTTP handler. Before the match starts, the first
// Need distinct names get seats in connection order. After that, only a known
// name that is currently disconnected may connect, and it takes its seat back.
type Hub struct {
	need int
	// Logf receives operator-facing messages. Defaults to discarding them.
	Logf func(format string, args ...any)

	mu      sync.Mutex
	started bool
	players []*RemotePlayer
	byName  map[string]*RemotePlayer
	ready   chan struct{}
}

// NewHub returns a hub that becomes ready once need bots have connected.
func NewHub(need int) *Hub {
	h := &Hub{need: need, byName: map[string]*RemotePlayer{}, ready: make(chan struct{})}
	if need == 0 {
		close(h.ready)
	}
	return h
}

func (h *Hub) logf(format string, args ...any) {
	if h.Logf != nil {
		h.Logf(format, args...)
	}
}

// Ready is closed once the needed number of bots have connected.
func (h *Hub) Ready() <-chan struct{} { return h.ready }

// Start closes registration and returns the players in connection order.
func (h *Hub) Start() []*RemotePlayer {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.started = true
	return append([]*RemotePlayer(nil), h.players...)
}

// Players returns the registered players in connection order.
func (h *Hub) Players() []*RemotePlayer {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]*RemotePlayer(nil), h.players...)
}

// ServeHTTP upgrades the request to a WebSocket and runs the bot connection
// until it closes.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Bots are not browsers; accept any origin.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	conn.SetReadLimit(1 << 16)

	ctx, cancel := context.WithTimeout(r.Context(), helloTimeout)
	var hello protocol.Hello
	err = wsjson.Read(ctx, conn, &hello)
	cancel()
	if err != nil || hello.Type != protocol.TypeHello {
		reject(conn, "first message must be hello")
		return
	}
	p, reconnect, msg := h.register(hello.Name)
	if p == nil {
		h.logf("rejected %q: %s", hello.Name, msg)
		reject(conn, msg)
		return
	}
	if err := write(conn, protocol.Welcome{Type: protocol.TypeWelcome, Name: p.name}); err != nil {
		conn.CloseNow()
		return
	}
	if !p.attach(conn) {
		reject(conn, "name taken")
		return
	}
	if reconnect {
		h.logf("%s reconnected", p.name)
	} else {
		h.logf("%s connected", p.name)
	}
	h.checkReady()

	go keepAlive(r.Context(), conn)
	p.readLoop(conn) // blocks until the connection drops
	h.logf("%s disconnected", p.name)
}

// register finds or creates the player for name. It returns nil and a reason
// if the name may not connect now.
func (h *Hub) register(name string) (p *RemotePlayer, reconnect bool, reason string) {
	if name == "" || len(name) > maxNameLen {
		return nil, false, fmt.Sprintf("name must be 1-%d bytes", maxNameLen)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if p := h.byName[name]; p != nil {
		if p.Connected() {
			return nil, false, "name taken"
		}
		return p, true, ""
	}
	if h.started {
		return nil, false, "match already started"
	}
	if len(h.players) >= h.need {
		return nil, false, "match is full"
	}
	p = newRemotePlayer(name)
	p.logf = h.logf
	h.players = append(h.players, p)
	h.byName[name] = p
	return p, false, ""
}

func (h *Hub) checkReady() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.players) == h.need {
		select {
		case <-h.ready:
		default:
			close(h.ready)
		}
	}
}

func reject(conn *websocket.Conn, msg string) {
	write(conn, protocol.Error{Type: protocol.TypeError, Message: msg})
	conn.Close(websocket.StatusPolicyViolation, msg)
}

func write(conn *websocket.Conn, v any) error {
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	return wsjson.Write(ctx, conn, v)
}

// keepAlive pings the bot so idle connections (e.g. while waiting for the
// match to start) are not dropped by proxies such as Cloudflare, and closes
// connections that stop answering.
func keepAlive(ctx context.Context, conn *websocket.Conn) {
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, pingTimeout)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				conn.CloseNow()
				return
			}
		}
	}
}

// decodeResponse parses a bot message. Anything that is not a well-formed
// response but carries an id is returned as an empty response for that id, so
// the runner records it as illegal instead of waiting for the timeout.
func decodeResponse(b []byte) (*protocol.Response, error) {
	var env struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	if env.Type != protocol.TypeResponse {
		return nil, fmt.Errorf("unexpected message type %q", env.Type)
	}
	if env.ID == "" {
		return nil, errors.New("response without id")
	}
	var resp protocol.Response
	if err := json.Unmarshal(b, &resp); err != nil {
		return &protocol.Response{Type: protocol.TypeResponse, ID: env.ID}, nil
	}
	return &resp, nil
}
