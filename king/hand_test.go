package king

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
)

// cs parses "QS TH 2C" into cards.
func cs(s string) []Card { return MustParseCards(s) }

func mustCard(s string) Card {
	c, err := ParseCard(s)
	if err != nil {
		panic(err)
	}
	return c
}

func setup(t *testing.T, c Contract, leader int, hands [4]string, plays string) *Hand {
	t.Helper()
	var cards [4][]Card
	for i, s := range hands {
		cards[i] = cs(s)
	}
	h := NewHand(cards, c, leader)
	for _, card := range cs(plays) {
		if err := h.Play(h.Turn(), card); err != nil {
			t.Fatalf("setup play %v by seat %d: %v", card, h.Turn(), err)
		}
	}
	return h
}

var (
	trumpS = Contract{Kind: Trump, Suit: Spades}
	noTr   = Contract{Kind: NoTricks}
	noH    = Contract{Kind: NoHearts}
	noQ    = Contract{Kind: NoQueens}
	noKJ   = Contract{Kind: NoKingsJacks}
	rifki  = Contract{Kind: NoKingOfHearts}
)

func TestLegal(t *testing.T) {
	tests := []struct {
		name     string
		contract Contract
		hands    [4]string
		plays    string
		want     string
	}{
		// Trump contracts.
		{"trump: must trump when void, underruff allowed (RULES example)", trumpS,
			[4]string{"AH 2C", "9S 2D", "3S QS 7D", "4C 5C"}, "AH 9S", "3S QS"},
		{"trump: void with no trumps plays anything", trumpS,
			[4]string{"AH 2C", "9S 2D", "7D 8C", "4C 5C"}, "AH 9S", "8C 7D"},
		{"trump: must follow suit even holding trumps", trumpS,
			[4]string{"AH 2C", "9S 2D", "5H QS", "4C 5C"}, "AH 9S", "5H"},
		{"trump: no must-beat when following", trumpS,
			[4]string{"7H", "5H JH", "2C", "3C"}, "7H", "5H JH"},
		{"trump: leading trump allowed any time", trumpS,
			[4]string{"KS 2C", "2D", "3D", "4D"}, "", "2C KS"},

		// Following suit in negative contracts.
		{"no_tricks: must follow suit", noTr,
			[4]string{"7S", "5S JS 2H", "2C", "3C"}, "7S", "5S JS"},
		{"no_tricks: void discards anything", noTr,
			[4]string{"2C", "KH 4H 8S", "3C", "4C"}, "2C", "4H KH 8S"},

		// N1: forced penalty play.
		{"N1 no_queens: queen under a played king (RULES example)", noQ,
			[4]string{"5D", "KD", "QD 3D", "2C"}, "5D KD", "QD"},
		{"N1 no_queens: nothing higher than the queen yet", noQ,
			[4]string{"5D", "9D", "QD 3D", "2C"}, "5D 9D", "3D QD"},
		{"N1 no_kings_jacks: any penalty card once one is covered (RULES example)", noKJ,
			[4]string{"4C", "QC", "KC JC 2C", "2D"}, "4C QC", "JC KC"},
		{"N1 no_kings_jacks: king alone is not covered by a queen", noKJ,
			[4]string{"4C", "QC", "KC 2C", "2D"}, "4C QC", "2C KC"},
		{"N1 no_hearts: any heart (RULES example)", noH,
			[4]string{"TH", "9H AH", "2C", "3C"}, "TH", "9H AH"},
		{"N1 rifki: K♥ under a played A♥", rifki,
			[4]string{"AH", "KH 2H", "2C", "3C"}, "AH", "KH"},

		// N2: forced penalty discard.
		{"N2 rifki: must discard K♥ (RULES example)", rifki,
			[4]string{"2C", "KH 4H 8S", "3C", "4C"}, "2C", "KH"},
		{"N2 no_hearts: any heart (RULES example)", noH,
			[4]string{"2C", "KH 4H 8S", "3C", "4C"}, "2C", "4H KH"},
		{"N2 no_queens: must discard a queen", noQ,
			[4]string{"2C", "QH QS 8S", "3C", "4C"}, "2C", "QH QS"},
		{"N2 no_queens: no queen, discard anything", noQ,
			[4]string{"2C", "KH 8S", "3C", "4C"}, "2C", "KH 8S"},

		// N3: heart-lead restriction.
		{"N3 no_hearts: cannot lead a heart before hearts are broken", noH,
			[4]string{"2H 3C", "2D", "3D", "4D"}, "", "3C"},
		{"N3 rifki: cannot lead a heart before hearts are broken", rifki,
			[4]string{"2H 3C", "2D", "3D", "4D"}, "", "3C"},
		{"N3 no_hearts: only hearts may be led", noH,
			[4]string{"2H 5H", "2D", "3D", "4D"}, "", "2H 5H"},
		{"N3 no_hearts: hearts may be led once discarded earlier", noH,
			[4]string{"2C 8D", "5H 9D", "3C TD", "4C 6H 7C"}, "2C 5H 3C 4C", "6H 7C"},
		{"N3 does not apply to no_queens", noQ,
			[4]string{"2H 3C", "2D", "3D", "4D"}, "", "3C 2H"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := setup(t, tt.contract, 0, tt.hands, tt.plays)
			got := h.Legal(h.Turn())
			want := cs(tt.want)
			SortHand(want)
			if !slices.Equal(got, want) {
				t.Fatalf("seat %d legal = %v, want %v", h.Turn(), got, want)
			}
		})
	}
}

func TestTrickWinnerAndScore(t *testing.T) {
	h := setup(t, trumpS, 0, [4]string{"AH 2C", "9S 2D", "3S QS", "KH 5C"}, "AH 9S 3S KH")
	tr := h.Tricks()
	if len(tr) != 1 || tr[0].Winner != 1 {
		t.Fatalf("tricks = %+v", tr)
	}
	if h.Score() != [4]int{0, 50, 0, 0} || h.Turn() != 1 {
		t.Fatalf("score %v turn %d", h.Score(), h.Turn())
	}

	h = setup(t, noQ, 0, [4]string{"5D 2C", "KD 3C", "QD 4C", "AS 5C"}, "5D KD QD AS")
	if h.Score() != [4]int{0, -100, 0, 0} {
		t.Fatalf("off-suit ace must not win: %v", h.Score())
	}
}

func TestEarlyEnd(t *testing.T) {
	h := setup(t, rifki, 0, [4]string{"2C 3D", "KH 4D", "5C 6D", "7C 8D"}, "2C KH 5C 7C")
	if !h.Done() || h.Score() != [4]int{0, 0, 0, -320} {
		t.Fatalf("done=%v score=%v", h.Done(), h.Score())
	}
	if h.Legal(h.Turn()) != nil {
		t.Fatal("legal moves after hand over")
	}
	if err := h.Play(3, mustCard("8D")); !errors.Is(err, ErrHandOver) {
		t.Fatalf("play after end: %v", err)
	}
}

func TestPlayErrors(t *testing.T) {
	h := setup(t, noTr, 0, [4]string{"2C 3D", "4C 5D", "6C", "7C"}, "")
	if err := h.Play(1, mustCard("4C")); !errors.Is(err, ErrNotYourTurn) {
		t.Fatalf("out of turn: %v", err)
	}
	if err := h.Play(0, mustCard("AS")); !errors.Is(err, ErrIllegalCard) {
		t.Fatalf("card not held: %v", err)
	}
	h.Play(0, mustCard("2C"))
	if err := h.Play(1, mustCard("5D")); !errors.Is(err, ErrIllegalCard) {
		t.Fatalf("revoke: %v", err)
	}
}

// Random legal play never gets stuck, and every hand hands out exactly the
// contract's total.
func TestRandomHands(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, c := range AllContracts() {
		want := 0
		for i := range 13 {
			want += c.TrickScore(i)
		}
		for _, card := range Deck() {
			want += c.CardPenalty(card)
		}
		for range 10000 {
			h := NewHand(Deal(r.Uint64()), c, r.IntN(4))
			for !h.Done() {
				legal := h.Legal(h.Turn())
				if len(legal) == 0 {
					t.Fatalf("%v: no legal move for seat %d", c, h.Turn())
				}
				if err := h.Play(h.Turn(), legal[r.IntN(len(legal))]); err != nil {
					t.Fatal(err)
				}
			}
			s := h.Score()
			if got := s[0] + s[1] + s[2] + s[3]; got != want {
				t.Fatalf("%v: hand total %d want %d", c, got, want)
			}
			if !c.Kind.CardBased() && len(h.Tricks()) != 13 {
				t.Fatalf("%v ended after %d tricks", c, len(h.Tricks()))
			}
		}
	}
}
