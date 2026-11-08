// Command examplebot is a minimal King bot. Copy it as a starting point.
//
//	examplebot -url ws://127.0.0.1:8080/ws -name alice -strategy random
//
// It answers every request using only the legal options the server sends,
// handles several requests at once, and reconnects with backoff under the
// same name if the connection drops. See PROTOCOL.md.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/cenkalti/king-server/king"
	"github.com/cenkalti/king-server/protocol"
)

var (
	url      = flag.String("url", "ws://127.0.0.1:8080/ws", "server WebSocket URL")
	name     = flag.String("name", "", "bot name (required)")
	strategy = flag.String("strategy", "random", "random or low")
	seed     = flag.Uint64("seed", 0, "random seed (0 = random)")
)

func main() {
	flag.Parse()
	if *name == "" {
		log.Fatal("-name is required")
	}
	log.SetFlags(log.Ltime)
	log.SetPrefix("[" + *name + "] ")
	if *seed == 0 {
		*seed = rand.Uint64()
	}
	b := &bot{r: rand.New(rand.NewPCG(*seed, *seed))}
	backoff := 100 * time.Millisecond
	for {
		end, err := b.run(context.Background())
		if end != nil {
			for _, s := range end.Standings {
				if s.Name == *name {
					fmt.Printf("%s finished #%d: %.1f placement points, %d points, %d strikes\n", s.Name, s.Rank, s.PlacementPoints, s.TotalPoints, s.Strikes)
				}
			}
			return
		}
		// A rejection is retried too: "name taken" right after a network drop
		// usually means the server hasn't noticed our old connection is dead.
		if errors.Is(err, errConnected) {
			backoff = 100 * time.Millisecond // we had a working connection
		}
		log.Printf("connection lost (%v); reconnecting in %s", err, backoff)
		time.Sleep(backoff)
		backoff = min(backoff*2, 5*time.Second)
	}
}

var errConnected = errors.New("was connected")

type bot struct {
	mu sync.Mutex // guards r
	r  *rand.Rand
}

// run connects, says hello and answers requests until the connection drops
// or the match ends.
func (b *bot) run(ctx context.Context) (*protocol.MatchEnd, error) {
	conn, _, err := websocket.Dial(ctx, *url, nil)
	if err != nil {
		return nil, err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)

	if err := wsjson.Write(ctx, conn, protocol.Hello{Type: protocol.TypeHello, Name: *name}); err != nil {
		return nil, err
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errConnected, err)
		}
		var env protocol.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		switch env.Type {
		case protocol.TypeWelcome:
			log.Printf("connected as %s", *name)
		case protocol.TypeError:
			var e protocol.Error
			json.Unmarshal(data, &e)
			return nil, fmt.Errorf("rejected: %s", e.Message)
		case protocol.TypeMatchEnd:
			var m protocol.MatchEnd
			json.Unmarshal(data, &m)
			return &m, nil
		case protocol.TypeRequest:
			var req protocol.Request
			if err := json.Unmarshal(data, &req); err != nil {
				continue
			}
			// Answer in a goroutine: in parallel mode several requests
			// can be outstanding. Conn writes are safe to call concurrently.
			go func() {
				resp := b.answer(&req)
				wsjson.Write(ctx, conn, resp)
			}()
		}
	}
}

// answer is where a real bot's strategy goes. req.State has the full game
// state; this example only looks at the legal options.
func (b *bot) answer(req *protocol.Request) protocol.Response {
	resp := protocol.Response{Type: protocol.TypeResponse, ID: req.ID}
	switch req.Kind {
	case protocol.ChooseContract:
		c := req.LegalContracts[0]
		if *strategy == "random" {
			c = req.LegalContracts[b.intN(len(req.LegalContracts))]
		}
		resp.Contract = &c
	case protocol.PlayCard:
		c := king.Lowest(req.LegalCards)
		if *strategy == "random" {
			c = req.LegalCards[b.intN(len(req.LegalCards))]
		}
		resp.Card = &c
	}
	return resp
}

func (b *bot) intN(n int) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.r.IntN(n)
}
