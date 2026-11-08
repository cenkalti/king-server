package match

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"

	"github.com/cenkalti/king-server/king"
	"github.com/cenkalti/king-server/protocol"
)

// Player makes decisions for one bot.
//
// Decide must return when ctx is done. A Player that has lost its connection
// returns an error wrapping ErrDisconnected; if it also implements
// Reconnector, the runner waits for it to come back and asks again.
type Player interface {
	Decide(ctx context.Context, req *protocol.Request) (*protocol.Response, error)
}

// ErrDisconnected means the player's connection is gone.
var ErrDisconnected = errors.New("player disconnected")

// Reconnector is implemented by players that can reconnect.
type Reconnector interface {
	// WaitConnected blocks until the player is connected or ctx is done.
	WaitConnected(ctx context.Context) error
}

func answer(req *protocol.Request, card *king.Card, contract *king.Contract) *protocol.Response {
	return &protocol.Response{Type: protocol.TypeResponse, ID: req.ID, Card: card, Contract: contract}
}

// RandomBot picks uniformly among the legal options.
type RandomBot struct {
	mu sync.Mutex
	r  *rand.Rand
}

func NewRandomBot(seed uint64) *RandomBot {
	return &RandomBot{r: rand.New(rand.NewPCG(seed, 0xbadc0ffee))}
}

func (b *RandomBot) Decide(_ context.Context, req *protocol.Request) (*protocol.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if req.Kind == protocol.ChooseContract {
		c := req.LegalContracts[b.r.IntN(len(req.LegalContracts))]
		return answer(req, nil, &c), nil
	}
	c := req.LegalCards[b.r.IntN(len(req.LegalCards))]
	return answer(req, &c, nil), nil
}

// LowBot plays its lowest legal card and declares the first legal contract.
// It plays exactly like the server's fault fallback.
type LowBot struct{}

func (LowBot) Decide(_ context.Context, req *protocol.Request) (*protocol.Response, error) {
	if req.Kind == protocol.ChooseContract {
		c := req.LegalContracts[0]
		return answer(req, nil, &c), nil
	}
	c := king.Lowest(req.LegalCards)
	return answer(req, &c, nil), nil
}
