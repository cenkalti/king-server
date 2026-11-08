package king

import (
	"encoding/json"
	"fmt"
)

// ContractKind identifies one of the seven contract types. The numeric order
// is the fixed order used by the "lowest legal contract" fallback.
type ContractKind uint8

const (
	NoTricks ContractKind = iota
	NoHearts
	NoQueens
	NoKingsJacks
	NoLastTwo
	NoKingOfHearts
	Trump
)

// NegativeKinds lists the six negative contracts in enum order.
var NegativeKinds = [6]ContractKind{NoTricks, NoHearts, NoQueens, NoKingsJacks, NoLastTwo, NoKingOfHearts}

var kindNames = [7]string{"no_tricks", "no_hearts", "no_queens", "no_kings_jacks", "no_last_two", "no_king_of_hearts", "trump"}

// KindNames returns the wire names of every contract kind in enum order.
func KindNames() []string { return kindNames[:] }

func (k ContractKind) Valid() bool { return k <= Trump }

func (k ContractKind) String() string {
	if !k.Valid() {
		return fmt.Sprintf("ContractKind(%d)", uint8(k))
	}
	return kindNames[k]
}

func (k ContractKind) MarshalText() ([]byte, error) {
	if !k.Valid() {
		return nil, fmt.Errorf("invalid contract kind %d", uint8(k))
	}
	return []byte(kindNames[k]), nil
}

func (k *ContractKind) UnmarshalText(b []byte) error {
	for i, n := range kindNames {
		if string(b) == n {
			*k = ContractKind(i)
			return nil
		}
	}
	return fmt.Errorf("invalid contract kind %q", b)
}

// IsNegative reports whether k is one of the six negative contracts.
func (k ContractKind) IsNegative() bool { return k < Trump }

// CardBased reports whether k penalizes specific cards (and so can end early).
func (k ContractKind) CardBased() bool {
	switch k {
	case NoHearts, NoQueens, NoKingsJacks, NoKingOfHearts:
		return true
	}
	return false
}

// Contract is what a declarer chooses for a hand. Suit is meaningful only when
// Kind is Trump.
type Contract struct {
	Kind ContractKind
	Suit Suit
}

// AllContracts lists every possible contract in fallback order: the six
// negatives, then trump in C, D, H, S.
func AllContracts() []Contract {
	cs := make([]Contract, 0, 10)
	for _, k := range NegativeKinds {
		cs = append(cs, Contract{Kind: k})
	}
	for _, s := range Suits {
		cs = append(cs, Contract{Kind: Trump, Suit: s})
	}
	return cs
}

func (c Contract) Valid() bool {
	return c.Kind.Valid() && (c.Kind != Trump || c.Suit.Valid())
}

func (c Contract) String() string {
	if c.Kind == Trump {
		return "trump " + c.Suit.String()
	}
	return c.Kind.String()
}

type contractJSON struct {
	Kind ContractKind `json:"kind"`
	Suit *Suit        `json:"suit,omitempty"`
}

// MarshalJSON encodes {"kind":"no_queens"} or {"kind":"trump","suit":"S"}.
func (c Contract) MarshalJSON() ([]byte, error) {
	j := contractJSON{Kind: c.Kind}
	if c.Kind == Trump {
		s := c.Suit
		j.Suit = &s
	}
	return json.Marshal(j)
}

func (c *Contract) UnmarshalJSON(b []byte) error {
	var j contractJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	switch {
	case j.Kind == Trump && j.Suit == nil:
		return fmt.Errorf("trump contract needs a suit")
	case j.Kind != Trump && j.Suit != nil:
		return fmt.Errorf("%s contract takes no suit", j.Kind)
	}
	*c = Contract{Kind: j.Kind}
	if j.Suit != nil {
		c.Suit = *j.Suit
	}
	return nil
}

// IsPenaltyCard reports whether card carries a penalty in contract c. It is
// always false for trick-based and trump contracts.
func (c Contract) IsPenaltyCard(card Card) bool {
	switch c.Kind {
	case NoHearts:
		return card.Suit == Hearts
	case NoQueens:
		return card.Rank == Queen
	case NoKingsJacks:
		return card.Rank == King || card.Rank == Jack
	case NoKingOfHearts:
		return card == Card{Rank: King, Suit: Hearts}
	}
	return false
}

// CardPenalty is the (negative) score for winning card in contract c, or 0.
func (c Contract) CardPenalty(card Card) int {
	if !c.IsPenaltyCard(card) {
		return 0
	}
	switch c.Kind {
	case NoHearts:
		return -30
	case NoQueens:
		return -100
	case NoKingsJacks:
		return -60
	case NoKingOfHearts:
		return -320
	}
	return 0
}

// TrickScore is the score for winning trick number i (0-based) in contract c,
// apart from any penalty cards in it: +50 in trump, −50 in no_tricks, −180 for
// tricks 12 and 13 in no_last_two, and 0 otherwise.
func (c Contract) TrickScore(i int) int {
	switch c.Kind {
	case Trump:
		return 50
	case NoTricks:
		return -50
	case NoLastTwo:
		if i >= 11 {
			return -180
		}
	}
	return 0
}

// PenaltyCardCount is the number of penalty cards in the deck for c.
func (c Contract) PenaltyCardCount() int {
	switch c.Kind {
	case NoHearts:
		return 13
	case NoQueens:
		return 4
	case NoKingsJacks:
		return 8
	case NoKingOfHearts:
		return 1
	}
	return 0
}
