package protocol

import "github.com/cenkalti/king-server/king"

// State is everything one seat is allowed to know when asked for a decision.
// It never contains other seats' cards except those already played.
type State struct {
	GameID int `json:"game_id"`
	// Seat is your seat, 0..3. Play is counter-clockwise: after seat s comes (s+1)%4.
	Seat int `json:"seat" jsonschema:"minimum=0,maximum=3"`
	// Players are the bot names by seat.
	Players []string `json:"players" jsonschema:"minItems=4,maxItems=4"`
	// HandNumber is 1..20.
	HandNumber int `json:"hand_number" jsonschema:"minimum=1,maximum=20"`
	Declarer   int `json:"declarer" jsonschema:"minimum=0,maximum=3"`
	// Contract is the current hand's contract; absent while it is being chosen.
	Contract *king.Contract `json:"contract,omitempty"`
	// Cards are the cards you hold now.
	Cards []king.Card `json:"cards"`
	// Tricks are the completed tricks of the current hand.
	Tricks []king.Trick `json:"tricks"`
	// CurrentTrick is the trick in progress (winner is -1).
	CurrentTrick king.Trick `json:"current_trick"`
	// History is the result of every earlier hand in this game.
	History []HandResult `json:"history"`
	// Totals are the game scores by seat, before the current hand.
	Totals []int `json:"totals" jsonschema:"minItems=4,maxItems=4"`
	// NegativeCounts is how many times each negative contract has been
	// declared in this game, keyed by contract kind.
	NegativeCounts map[string]int `json:"negative_counts"`
	// Quotas are the declarations each seat has left, by seat.
	Quotas []Quota `json:"quotas" jsonschema:"minItems=4,maxItems=4"`
}

// HandResult summarizes a finished hand.
type HandResult struct {
	HandNumber int           `json:"hand_number"`
	Declarer   int           `json:"declarer"`
	Contract   king.Contract `json:"contract"`
	Scores     []int         `json:"scores" jsonschema:"minItems=4,maxItems=4"`
}

// Quota is how many declarations of each type a seat has left.
type Quota struct {
	Trump    int `json:"trump"`
	Negative int `json:"negative"`
}

// Snapshot builds the state of game g as seen from seat.
func Snapshot(g *king.Game, gameID, seat int, players [4]string) State {
	st := State{
		GameID:         gameID,
		Seat:           seat,
		Players:        players[:],
		HandNumber:     g.HandNumber(),
		Declarer:       g.Declarer(),
		Cards:          g.Cards(seat),
		Tricks:         []king.Trick{},
		CurrentTrick:   king.Trick{Leader: g.Declarer(), Cards: []king.Card{}, Winner: -1},
		History:        []HandResult{},
		NegativeCounts: map[string]int{},
	}
	if st.Cards == nil {
		st.Cards = []king.Card{}
	}
	if h := g.Hand(); h != nil {
		c := h.Contract()
		st.Contract = &c
		if t := h.Tricks(); t != nil {
			st.Tricks = t
		}
		st.CurrentTrick = h.Current()
		if st.CurrentTrick.Cards == nil {
			st.CurrentTrick.Cards = []king.Card{}
		}
	}
	for _, rec := range g.History() {
		st.History = append(st.History, HandResult{
			HandNumber: rec.Number,
			Declarer:   rec.Declarer,
			Contract:   rec.Contract,
			Scores:     rec.Score[:],
		})
	}
	tot := g.Totals()
	st.Totals = tot[:]
	for _, k := range king.NegativeKinds {
		st.NegativeCounts[k.String()] = g.NegativeCount(k)
	}
	for s := range 4 {
		t, n := g.QuotaLeft(s)
		st.Quotas = append(st.Quotas, Quota{Trump: t, Negative: n})
	}
	return st
}

// NewRequest builds the request for whoever's turn it is in g.
func NewRequest(id string, g *king.Game, gameID int, players [4]string) *Request {
	seat := g.Turn()
	r := &Request{
		Type:   TypeRequest,
		ID:     id,
		GameID: gameID,
		State:  Snapshot(g, gameID, seat, players),
	}
	if g.Phase() == king.Choosing {
		r.Kind = ChooseContract
		r.LegalContracts = g.LegalContracts(seat)
	} else {
		r.Kind = PlayCard
		r.LegalCards = g.Legal(seat)
	}
	return r
}
