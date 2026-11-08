package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/cenkalti/king-server/king"
	"github.com/cenkalti/king-server/match"
	"github.com/cenkalti/king-server/protocol"
)

func newTestHub(t *testing.T, need int) (*Hub, string) {
	t.Helper()
	h := NewHub(need)
	// Hub goroutines may log after the test returns, when t.Logf panics.
	var mu sync.Mutex
	done := false
	t.Cleanup(func() { mu.Lock(); done = true; mu.Unlock() })
	h.Logf = func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		if !done {
			t.Logf(format, args...)
		}
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return h, "ws" + strings.TrimPrefix(srv.URL, "http")
}

// dial connects and sends hello. It returns the connection and the first
// message type the server sent back.
func dial(t *testing.T, url, name string) (*websocket.Conn, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	if err := wsjson.Write(ctx, c, protocol.Hello{Type: protocol.TypeHello, Name: name}); err != nil {
		t.Fatal(err)
	}
	var env protocol.Envelope
	if err := wsjson.Read(ctx, c, &env); err != nil {
		t.Fatal(err)
	}
	return c, env.Type
}

// answerLow answers every request with the lowest legal option until the
// connection closes.
func answerLow(c *websocket.Conn) {
	ctx := context.Background()
	for {
		var req protocol.Request
		if err := wsjson.Read(ctx, c, &req); err != nil {
			return
		}
		resp, _ := match.LowBot{}.Decide(ctx, &req)
		if err := wsjson.Write(ctx, c, resp); err != nil {
			return
		}
	}
}

func testRequest(id string) *protocol.Request {
	g := king.NewGame(king.Deals(1))
	return protocol.NewRequest(id, g, 0, [4]string{"a", "b", "c", "d"})
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestHandshakeAndReady(t *testing.T) {
	h, url := newTestHub(t, 4)
	for i, name := range []string{"a", "b", "c"} {
		if _, typ := dial(t, url, name); typ != protocol.TypeWelcome {
			t.Fatalf("bot %d got %s", i, typ)
		}
	}
	if _, typ := dial(t, url, "b"); typ != protocol.TypeError {
		t.Fatalf("duplicate name got %s", typ)
	}
	select {
	case <-h.Ready():
		t.Fatal("ready with 3 bots")
	default:
	}
	dial(t, url, "d")
	select {
	case <-h.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("not ready with 4 bots")
	}
	var order []string
	for _, p := range h.Start() {
		order = append(order, p.Name())
	}
	if strings.Join(order, ",") != "a,b,c,d" {
		t.Fatalf("seat order %v", order)
	}
	if _, typ := dial(t, url, "e"); typ != protocol.TypeError {
		t.Fatalf("unknown name after start got %s", typ)
	}
	if _, typ := dial(t, url, ""); typ != protocol.TypeError {
		t.Fatalf("empty name got %s", typ)
	}
}

func TestDecideAndReconnect(t *testing.T) {
	h, url := newTestHub(t, 1)
	c, _ := dial(t, url, "a")
	<-h.Ready()
	p := h.Start()[0]

	go answerLow(c)
	resp, err := p.Decide(context.Background(), testRequest("r1"))
	if err != nil || resp.ID != "r1" || resp.Contract == nil {
		t.Fatalf("resp %+v err %v", resp, err)
	}

	// Drop the connection while a request is pending.
	silent, _ := dial(t, url, "z") // rejected: hub is full and started
	silent.CloseNow()
	c.CloseNow()
	waitFor(t, func() bool { return !p.Connected() })
	if _, err := p.Decide(context.Background(), testRequest("r2")); !errors.Is(err, match.ErrDisconnected) {
		t.Fatalf("decide while disconnected: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.WaitConnected(waitCtx) }()
	c2, typ := dial(t, url, "a")
	if typ != protocol.TypeWelcome {
		t.Fatalf("reconnect got %s", typ)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	go answerLow(c2)
	if resp, err := p.Decide(context.Background(), testRequest("r2")); err != nil || resp.ID != "r2" {
		t.Fatalf("after reconnect: %+v %v", resp, err)
	}
}

func TestDropDuringPendingRequest(t *testing.T) {
	h, url := newTestHub(t, 1)
	c, _ := dial(t, url, "a")
	<-h.Ready()
	p := h.Start()[0]
	go func() {
		var req protocol.Request
		wsjson.Read(context.Background(), c, &req)
		c.CloseNow() // read the request, then vanish
	}()
	if _, err := p.Decide(context.Background(), testRequest("r1")); !errors.Is(err, match.ErrDisconnected) {
		t.Fatalf("got %v", err)
	}
}

func TestLateAndMalformedResponses(t *testing.T) {
	h, url := newTestHub(t, 1)
	c, _ := dial(t, url, "a")
	<-h.Ready()
	p := h.Start()[0]
	ctx := context.Background()

	// Timeout: the bot answers late; the answer is ignored.
	reqs := make(chan protocol.Request, 4)
	go func() {
		for {
			var req protocol.Request
			if err := wsjson.Read(ctx, c, &req); err != nil {
				return
			}
			reqs <- req
		}
	}()
	tctx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	_, err := p.Decide(tctx, testRequest("slow"))
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	<-reqs
	wsjson.Write(ctx, c, protocol.Response{Type: protocol.TypeResponse, ID: "slow"})

	// Malformed response with a valid id comes back as an empty response.
	done := make(chan *protocol.Response, 1)
	go func() {
		resp, _ := p.Decide(ctx, testRequest("bad"))
		done <- resp
	}()
	<-reqs
	c.Write(ctx, websocket.MessageText, []byte(`{"type":"response","id":"bad","card":"QS"}`))
	resp := <-done
	if resp == nil || resp.ID != "bad" || resp.Card != nil || resp.Contract != nil {
		t.Fatalf("malformed response decoded as %+v", resp)
	}
	if !p.Connected() {
		t.Fatal("bot disconnected by a late or malformed response")
	}
}

// End to end: a 4-game match over real WebSockets.
func TestMatchOverWebSocket(t *testing.T) {
	h, url := newTestHub(t, 4)
	names := []string{"a", "b", "c", "d"}
	for _, n := range names {
		c, _ := dial(t, url, n)
		go answerLow(c)
	}
	<-h.Ready()
	var r match.Runner
	for i, p := range h.Start() {
		r.Names[i] = p.Name()
		r.Players[i] = p
	}
	r.Games, r.Seed, r.Timeout = 4, 1, 2*time.Second
	st, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range st.Table() {
		if row.Strikes != 0 {
			t.Fatalf("strikes over websocket: %+v", row)
		}
	}
}
