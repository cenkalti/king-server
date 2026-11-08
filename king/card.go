// Package king implements the rules of Turkish King (Rıfkı) as specified in
// RULES.md. It is pure: no I/O, no clocks, no global randomness. Given the
// same deals and the same decisions it always produces the same game.
//
// Seats are numbered 0..3. Play is counter-clockwise, and the seat after s is
// (s+1)%4.
package king

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
)

// Suit is a card suit. The numeric order C < D < H < S breaks ties when the
// server picks a "lowest legal" card.
type Suit uint8

const (
	Clubs Suit = iota
	Diamonds
	Hearts
	Spades
)

// Suits lists every suit in tie-break order.
var Suits = [4]Suit{Clubs, Diamonds, Hearts, Spades}

var suitLetters = [4]string{"C", "D", "H", "S"}

func (s Suit) Valid() bool { return s <= Spades }

func (s Suit) String() string {
	if !s.Valid() {
		return fmt.Sprintf("Suit(%d)", uint8(s))
	}
	return suitLetters[s]
}

// MarshalText encodes the suit as its wire letter: C, D, H or S.
func (s Suit) MarshalText() ([]byte, error) {
	if !s.Valid() {
		return nil, fmt.Errorf("invalid suit %d", uint8(s))
	}
	return []byte(suitLetters[s]), nil
}

func (s *Suit) UnmarshalText(b []byte) error {
	p, err := ParseSuit(string(b))
	if err != nil {
		return err
	}
	*s = p
	return nil
}

// ParseSuit parses a wire suit letter.
func ParseSuit(v string) (Suit, error) {
	for i, l := range suitLetters {
		if v == l {
			return Suit(i), nil
		}
	}
	return 0, fmt.Errorf("invalid suit %q", v)
}

// Ranks of the face cards. Number cards use their face value (2..10).
const (
	Jack  = 11
	Queen = 12
	King  = 13
	Ace   = 14
)

// Card is a playing card. Rank is 2..14.
type Card struct {
	Rank int  `json:"rank" jsonschema:"minimum=2,maximum=14"`
	Suit Suit `json:"suit"`
}

func (c Card) Valid() bool { return c.Rank >= 2 && c.Rank <= Ace && c.Suit.Valid() }

var rankNames = map[int]string{10: "T", Jack: "J", Queen: "Q", King: "K", Ace: "A"}

// String gives a compact form such as "QS" or "TH". It is for logs and test
// output only; the wire uses {"rank","suit"} objects.
func (c Card) String() string {
	r, ok := rankNames[c.Rank]
	if !ok {
		r = fmt.Sprint(c.Rank)
	}
	return r + c.Suit.String()
}

// ParseCard parses the compact form produced by String, e.g. "QS", "TH", "2C".
func ParseCard(v string) (Card, error) {
	if len(v) != 2 {
		return Card{}, fmt.Errorf("invalid card %q", v)
	}
	rank := 0
	for r, n := range rankNames {
		if n == v[:1] {
			rank = r
		}
	}
	if rank == 0 && v[0] >= '2' && v[0] <= '9' {
		rank = int(v[0] - '0')
	}
	suit, err := ParseSuit(v[1:])
	if rank == 0 || err != nil {
		return Card{}, fmt.Errorf("invalid card %q", v)
	}
	return Card{Rank: rank, Suit: suit}, nil
}

// MustParseCards parses space-separated cards such as "QS TH 2C". It panics
// on invalid input and is meant for tests and fixed scenarios.
func MustParseCards(s string) []Card {
	var out []Card
	for f := range strings.FieldsSeq(s) {
		c, err := ParseCard(f)
		if err != nil {
			panic(err)
		}
		out = append(out, c)
	}
	return out
}

// Less orders cards by rank, then by suit (C < D < H < S). It defines the
// "lowest legal card" fallback.
func Less(a, b Card) bool {
	if a.Rank != b.Rank {
		return a.Rank < b.Rank
	}
	return a.Suit < b.Suit
}

// Lowest returns the lowest card by Less. It panics on an empty slice.
func Lowest(cards []Card) Card {
	return slices.MinFunc(cards, compareLow)
}

func compareLow(a, b Card) int {
	switch {
	case Less(a, b):
		return -1
	case Less(b, a):
		return 1
	}
	return 0
}

// SortHand sorts cards for display: by suit, then by rank.
func SortHand(cards []Card) {
	slices.SortFunc(cards, func(a, b Card) int {
		if a.Suit != b.Suit {
			return int(a.Suit) - int(b.Suit)
		}
		return a.Rank - b.Rank
	})
}

// Deck returns all 52 cards in a fixed order.
func Deck() []Card {
	d := make([]Card, 0, 52)
	for _, s := range Suits {
		for r := 2; r <= Ace; r++ {
			d = append(d, Card{Rank: r, Suit: s})
		}
	}
	return d
}

// Deal shuffles a deck with the given seed and deals 13 cards to each seat.
// The same seed always gives the same deal. Each hand is sorted with SortHand.
func Deal(seed uint64) [4][]Card {
	d := Deck()
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	r.Shuffle(len(d), func(i, j int) { d[i], d[j] = d[j], d[i] })
	var hands [4][]Card
	for s := range hands {
		hands[s] = slices.Clone(d[s*13 : (s+1)*13])
		SortHand(hands[s])
	}
	return hands
}
