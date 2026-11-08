package king

import (
	"errors"
	"fmt"
	"slices"
)

// Trick is one round of up to four cards. Cards are in play order, starting
// with Leader, so Cards[i] was played by seat (Leader+i)%4.
type Trick struct {
	Leader int    `json:"leader"`
	Cards  []Card `json:"cards"`
	// Winner is the seat that won the trick, or -1 while it is incomplete.
	Winner int `json:"winner"`
}

// Seat returns the seat that played Cards[i].
func (t Trick) Seat(i int) int { return (t.Leader + i) % 4 }

var (
	ErrNotYourTurn = errors.New("not your turn")
	ErrHandOver    = errors.New("hand is over")
	ErrIllegalCard = errors.New("illegal card")
)

// Hand is one hand of play under a single contract.
type Hand struct {
	contract Contract
	cards    [4][]Card
	tricks   []Trick
	current  Trick
	// heartsBroken is set once a heart has been played on a completed trick.
	heartsBroken  bool
	penaltyFallen int
	done          bool
	score         [4]int
}

// NewHand starts a hand. leader leads the first trick. The cards slices are
// copied.
func NewHand(cards [4][]Card, c Contract, leader int) *Hand {
	h := &Hand{contract: c, current: Trick{Leader: leader, Winner: -1}}
	for s := range cards {
		h.cards[s] = slices.Clone(cards[s])
		SortHand(h.cards[s])
	}
	return h
}

func (h *Hand) Contract() Contract { return h.contract }

// Cards returns a copy of the cards seat still holds.
func (h *Hand) Cards(seat int) []Card { return slices.Clone(h.cards[seat]) }

// Tricks returns the completed tricks.
func (h *Hand) Tricks() []Trick { return slices.Clone(h.tricks) }

// Current returns the trick in progress.
func (h *Hand) Current() Trick {
	t := h.current
	t.Cards = slices.Clone(t.Cards)
	return t
}

// Turn returns the seat that must play next. It is meaningless once Done.
func (h *Hand) Turn() int { return h.current.Seat(len(h.current.Cards)) }

// Done reports whether the hand is over: 13 tricks played, or every penalty
// card of a card-based contract has fallen.
func (h *Hand) Done() bool { return h.done }

// Score returns each seat's score for this hand so far.
func (h *Hand) Score() [4]int { return h.score }

// Legal returns the cards seat may play now, in SortHand order. It returns nil
// if the hand is over or it is not seat's turn.
func (h *Hand) Legal(seat int) []Card {
	if h.done || seat != h.Turn() {
		return nil
	}
	hand := h.cards[seat]
	if len(h.current.Cards) == 0 {
		return h.legalLead(hand)
	}
	led := h.current.Cards[0].Suit
	follow := filter(hand, func(c Card) bool { return c.Suit == led })
	if len(follow) > 0 {
		return h.legalFollow(follow, led)
	}
	return h.legalVoid(hand)
}

func (h *Hand) legalLead(hand []Card) []Card {
	k := h.contract.Kind
	if (k == NoHearts || k == NoKingOfHearts) && !h.heartsBroken {
		nonHearts := filter(hand, func(c Card) bool { return c.Suit != Hearts })
		if len(nonHearts) > 0 {
			return nonHearts
		}
	}
	return slices.Clone(hand)
}

// legalFollow applies rule N1: when following in a card-based contract, if a
// card of the led suit higher than one of your penalty cards of that suit has
// been played, you must play a penalty card of that suit.
func (h *Hand) legalFollow(follow []Card, led Suit) []Card {
	if !h.contract.Kind.CardBased() {
		return follow
	}
	high := 0
	for _, c := range h.current.Cards {
		if c.Suit == led && c.Rank > high {
			high = c.Rank
		}
	}
	penalty := filter(follow, h.contract.IsPenaltyCard)
	if slices.ContainsFunc(penalty, func(c Card) bool { return c.Rank < high }) {
		return penalty
	}
	return follow
}

// legalVoid handles not being able to follow suit: forced trump in trump
// contracts, and rule N2 (forced penalty discard) in card-based contracts.
func (h *Hand) legalVoid(hand []Card) []Card {
	var forced []Card
	switch {
	case h.contract.Kind == Trump:
		forced = filter(hand, func(c Card) bool { return c.Suit == h.contract.Suit })
	case h.contract.Kind.CardBased():
		forced = filter(hand, h.contract.IsPenaltyCard)
	}
	if len(forced) > 0 {
		return forced
	}
	return slices.Clone(hand)
}

// Play plays card for seat. It fails if it is not seat's turn, the hand is
// over, or the card is not in Legal(seat).
func (h *Hand) Play(seat int, card Card) error {
	if h.done {
		return ErrHandOver
	}
	if seat != h.Turn() {
		return ErrNotYourTurn
	}
	if !slices.Contains(h.Legal(seat), card) {
		return fmt.Errorf("%w: %v", ErrIllegalCard, card)
	}
	i := slices.Index(h.cards[seat], card)
	h.cards[seat] = slices.Delete(h.cards[seat], i, i+1)
	h.current.Cards = append(h.current.Cards, card)
	if len(h.current.Cards) == 4 {
		h.finishTrick()
	}
	return nil
}

func (h *Hand) finishTrick() {
	t := h.current
	t.Winner = t.Seat(h.trickWinner(t))
	h.score[t.Winner] += h.contract.TrickScore(len(h.tricks))
	for _, c := range t.Cards {
		h.score[t.Winner] += h.contract.CardPenalty(c)
		if h.contract.IsPenaltyCard(c) {
			h.penaltyFallen++
		}
		if c.Suit == Hearts {
			h.heartsBroken = true
		}
	}
	h.tricks = append(h.tricks, t)
	h.current = Trick{Leader: t.Winner, Winner: -1}
	if len(h.tricks) == 13 || (h.contract.Kind.CardBased() && h.penaltyFallen == h.contract.PenaltyCardCount()) {
		h.done = true
	}
}

// trickWinner returns the index into t.Cards of the winning card.
func (h *Hand) trickWinner(t Trick) int {
	suit := t.Cards[0].Suit
	if h.contract.Kind == Trump && slices.ContainsFunc(t.Cards, func(c Card) bool { return c.Suit == h.contract.Suit }) {
		suit = h.contract.Suit
	}
	best := -1
	for i, c := range t.Cards {
		if c.Suit == suit && (best < 0 || c.Rank > t.Cards[best].Rank) {
			best = i
		}
	}
	return best
}

func filter(cards []Card, keep func(Card) bool) []Card {
	var out []Card
	for _, c := range cards {
		if keep(c) {
			out = append(out, c)
		}
	}
	return out
}
