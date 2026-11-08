package king

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
)

// HandsPerGame is the number of hands in one game.
const HandsPerGame = 20

// Per-player declaration quotas.
const (
	TrumpQuota    = 2
	NegativeQuota = 3
	// MaxPerNegative is how many times each negative contract may be declared
	// per game.
	MaxPerNegative = 2
)

// Phase is what the game is waiting for.
type Phase int

const (
	Choosing Phase = iota // the declarer must choose a contract
	Playing               // a card must be played
	Over                  // all 20 hands are done
)

func (p Phase) String() string {
	return [...]string{"choosing", "playing", "over"}[p]
}

var ErrWrongPhase = errors.New("wrong phase")

// HandRecord is a finished hand.
type HandRecord struct {
	Number   int      `json:"number"` // 1..20
	Declarer int      `json:"declarer"`
	Contract Contract `json:"contract"`
	Tricks   []Trick  `json:"tricks"`
	Score    [4]int   `json:"score"`
}

// Game is a full 20-hand game.
type Game struct {
	deals         [HandsPerGame][4][]Card
	firstDeclarer int
	index         int // 0-based index of the current hand
	phase         Phase
	hand          *Hand
	history       []HandRecord
	totals        [4]int
	trumpUsed     [4]int
	negativeUsed  [4]int
	kindCount     [6]int
}

// Deals derives the 20 deals of one game from a seed.
func Deals(seed uint64) [HandsPerGame][4][]Card {
	r := rand.New(rand.NewPCG(seed, ^seed))
	var d [HandsPerGame][4][]Card
	for i := range d {
		d[i] = Deal(r.Uint64())
	}
	return d
}

// NewGame starts a game with the given deals, one per hand, indexed by seat.
// The holder of the 2♦ in the first deal declares first.
func NewGame(deals [HandsPerGame][4][]Card) *Game {
	g := &Game{deals: deals, firstDeclarer: -1}
	for s, cards := range deals[0] {
		if slices.Contains(cards, Card{Rank: 2, Suit: Diamonds}) {
			g.firstDeclarer = s
		}
	}
	if g.firstDeclarer < 0 {
		panic("king: first deal has no 2♦")
	}
	return g
}

func (g *Game) Phase() Phase { return g.phase }
func (g *Game) Done() bool   { return g.phase == Over }

// HandNumber is the current hand, 1..20 (21 once the game is over).
func (g *Game) HandNumber() int { return g.index + 1 }

// Declarer is the declarer of the current hand.
func (g *Game) Declarer() int { return (g.firstDeclarer + g.index) % 4 }

// FirstDeclarer is the seat that held the 2♦ in hand 1.
func (g *Game) FirstDeclarer() int { return g.firstDeclarer }

// Turn is the seat the game is waiting on, or -1 if the game is over.
func (g *Game) Turn() int {
	switch g.phase {
	case Choosing:
		return g.Declarer()
	case Playing:
		return g.hand.Turn()
	}
	return -1
}

// Hand returns the hand in progress, or nil while choosing or after the end.
func (g *Game) Hand() *Hand {
	if g.phase != Playing {
		return nil
	}
	return g.hand
}

// Cards returns the cards seat holds right now. While a contract is being
// chosen, that is the full deal of the coming hand.
func (g *Game) Cards(seat int) []Card {
	switch g.phase {
	case Choosing:
		return slices.Clone(g.deals[g.index][seat])
	case Playing:
		return g.hand.Cards(seat)
	}
	return nil
}

// History returns every finished hand.
func (g *Game) History() []HandRecord { return slices.Clone(g.history) }

// Totals returns each seat's game score so far.
func (g *Game) Totals() [4]int { return g.totals }

// QuotaLeft returns how many trump and negative declarations seat has left.
func (g *Game) QuotaLeft(seat int) (trump, negative int) {
	return TrumpQuota - g.trumpUsed[seat], NegativeQuota - g.negativeUsed[seat]
}

// NegativeCount returns how many times negative contract k has been declared.
func (g *Game) NegativeCount(k ContractKind) int {
	if !k.IsNegative() {
		return 0
	}
	return g.kindCount[k]
}

// LegalContracts returns the contracts seat may declare now, in fallback
// order. It returns nil unless seat is the declarer and a contract is due.
func (g *Game) LegalContracts(seat int) []Contract {
	if g.phase != Choosing || seat != g.Declarer() {
		return nil
	}
	trumpLeft, negLeft := g.QuotaLeft(seat)
	var out []Contract
	for _, c := range AllContracts() {
		if c.Kind == Trump && trumpLeft > 0 || c.Kind.IsNegative() && negLeft > 0 && g.kindCount[c.Kind] < MaxPerNegative {
			out = append(out, c)
		}
	}
	return out
}

// Choose declares contract c for the current hand.
func (g *Game) Choose(seat int, c Contract) error {
	if g.phase != Choosing {
		return ErrWrongPhase
	}
	if seat != g.Declarer() {
		return ErrNotYourTurn
	}
	if !slices.Contains(g.LegalContracts(seat), c) {
		return fmt.Errorf("illegal contract %v", c)
	}
	if c.Kind == Trump {
		g.trumpUsed[seat]++
	} else {
		g.negativeUsed[seat]++
		g.kindCount[c.Kind]++
	}
	g.hand = NewHand(g.deals[g.index], c, seat)
	g.phase = Playing
	return nil
}

// Legal returns the cards seat may play now (see Hand.Legal).
func (g *Game) Legal(seat int) []Card {
	if g.phase != Playing {
		return nil
	}
	return g.hand.Legal(seat)
}

// Play plays a card. When it finishes a hand, the game records the hand and
// moves to the next one.
func (g *Game) Play(seat int, card Card) error {
	if g.phase != Playing {
		return ErrWrongPhase
	}
	if err := g.hand.Play(seat, card); err != nil {
		return err
	}
	if !g.hand.Done() {
		return nil
	}
	score := g.hand.Score()
	g.history = append(g.history, HandRecord{
		Number:   g.index + 1,
		Declarer: g.Declarer(),
		Contract: g.hand.Contract(),
		Tricks:   g.hand.Tricks(),
		Score:    score,
	})
	for s := range g.totals {
		g.totals[s] += score[s]
	}
	g.index++
	g.hand = nil
	if g.index == HandsPerGame {
		g.phase = Over
	} else {
		g.phase = Choosing
	}
	return nil
}
