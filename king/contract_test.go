package king

import (
	"encoding/json"
	"testing"
)

func TestPenalties(t *testing.T) {
	kh := Card{King, Hearts}
	tests := []struct {
		kind  ContractKind
		card  Card
		want  int
		count int
	}{
		{NoHearts, Card{2, Hearts}, -30, 13},
		{NoHearts, Card{Queen, Spades}, 0, 13},
		{NoQueens, Card{Queen, Clubs}, -100, 4},
		{NoQueens, Card{King, Clubs}, 0, 4},
		{NoKingsJacks, Card{King, Diamonds}, -60, 8},
		{NoKingsJacks, Card{Jack, Spades}, -60, 8},
		{NoKingsJacks, Card{Queen, Spades}, 0, 8},
		{NoKingOfHearts, kh, -320, 1},
		{NoKingOfHearts, Card{King, Spades}, 0, 1},
		{NoTricks, kh, 0, 0},
		{NoLastTwo, kh, 0, 0},
		{Trump, kh, 0, 0},
	}
	for _, tt := range tests {
		c := Contract{Kind: tt.kind}
		if got := c.CardPenalty(tt.card); got != tt.want {
			t.Errorf("%s %v: penalty %d want %d", tt.kind, tt.card, got, tt.want)
		}
		if c.IsPenaltyCard(tt.card) != (tt.want != 0) {
			t.Errorf("%s %v: IsPenaltyCard mismatch", tt.kind, tt.card)
		}
		if got := c.PenaltyCardCount(); got != tt.count {
			t.Errorf("%s: count %d want %d", tt.kind, got, tt.count)
		}
	}
}

// The maximum total for each contract must match RULES.md.
func TestContractTotals(t *testing.T) {
	want := map[ContractKind]int{
		NoTricks: -650, NoHearts: -390, NoQueens: -400, NoKingsJacks: -480,
		NoLastTwo: -360, NoKingOfHearts: -320, Trump: 650,
	}
	for k, w := range want {
		c := Contract{Kind: k}
		total := 0
		for i := range 13 {
			total += c.TrickScore(i)
		}
		for _, card := range Deck() {
			total += c.CardPenalty(card)
		}
		if total != w {
			t.Errorf("%s total %d want %d", k, total, w)
		}
	}
}

func TestContractJSON(t *testing.T) {
	tests := []struct {
		c    Contract
		wire string
	}{
		{Contract{Kind: NoQueens}, `{"kind":"no_queens"}`},
		{Contract{Kind: Trump, Suit: Spades}, `{"kind":"trump","suit":"S"}`},
		{Contract{Kind: NoTricks}, `{"kind":"no_tricks"}`},
	}
	for _, tt := range tests {
		b, err := json.Marshal(tt.c)
		if err != nil || string(b) != tt.wire {
			t.Errorf("%v: got %s %v", tt.c, b, err)
		}
		var back Contract
		if err := json.Unmarshal([]byte(tt.wire), &back); err != nil || back != tt.c {
			t.Errorf("%s: got %v %v", tt.wire, back, err)
		}
	}
	for _, bad := range []string{`{"kind":"trump"}`, `{"kind":"no_queens","suit":"S"}`, `{"kind":"rifki"}`} {
		var c Contract
		if err := json.Unmarshal([]byte(bad), &c); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestAllContractsOrder(t *testing.T) {
	cs := AllContracts()
	if len(cs) != 10 || cs[0].Kind != NoTricks || cs[5].Kind != NoKingOfHearts || cs[6] != (Contract{Trump, Clubs}) || cs[9] != (Contract{Trump, Spades}) {
		t.Fatalf("unexpected order %v", cs)
	}
}
