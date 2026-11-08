package king

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// playRandom plays a full game with random legal decisions.
func playRandom(t *testing.T, g *Game, r *rand.Rand) {
	t.Helper()
	for !g.Done() {
		seat := g.Turn()
		switch g.Phase() {
		case Choosing:
			legal := g.LegalContracts(seat)
			if len(legal) == 0 {
				t.Fatalf("hand %d: no legal contract for seat %d", g.HandNumber(), seat)
			}
			if err := g.Choose(seat, legal[r.IntN(len(legal))]); err != nil {
				t.Fatal(err)
			}
		case Playing:
			legal := g.Legal(seat)
			if err := g.Play(seat, legal[r.IntN(len(legal))]); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestRandomGames(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 7))
	for range 1000 {
		g := NewGame(Deals(r.Uint64()))
		playRandom(t, g, r)

		h := g.History()
		if len(h) != HandsPerGame {
			t.Fatalf("%d hands", len(h))
		}
		var trumps, negs [4]int
		kinds := map[ContractKind]int{}
		for i, rec := range h {
			if want := (g.FirstDeclarer() + i) % 4; rec.Declarer != want {
				t.Fatalf("hand %d declarer %d want %d", i+1, rec.Declarer, want)
			}
			if rec.Contract.Kind == Trump {
				trumps[rec.Declarer]++
			} else {
				negs[rec.Declarer]++
				kinds[rec.Contract.Kind]++
			}
		}
		for s := range 4 {
			if trumps[s] != TrumpQuota || negs[s] != NegativeQuota {
				t.Fatalf("seat %d declared %d trump, %d negative", s, trumps[s], negs[s])
			}
		}
		for k, n := range kinds {
			if n > MaxPerNegative {
				t.Fatalf("%v declared %d times", k, n)
			}
		}
		tot := g.Totals()
		if sum := tot[0] + tot[1] + tot[2] + tot[3]; sum != 0 {
			t.Fatalf("game total %d, want 0", sum)
		}
	}
}

func TestFirstDeclarerHoldsTwoOfDiamonds(t *testing.T) {
	for seed := range uint64(50) {
		d := Deals(seed)
		g := NewGame(d)
		if !slices.Contains(d[0][g.FirstDeclarer()], Card{2, Diamonds}) {
			t.Fatalf("seed %d: first declarer %d lacks 2♦", seed, g.FirstDeclarer())
		}
		if g.Turn() != g.FirstDeclarer() || g.Phase() != Choosing {
			t.Fatalf("seed %d: game does not start with the 2♦ holder choosing", seed)
		}
	}
}

func TestChooseValidation(t *testing.T) {
	g := NewGame(Deals(1))
	d := g.Declarer()
	if err := g.Choose((d+1)%4, Contract{Kind: NoTricks}); err == nil {
		t.Fatal("non-declarer chose")
	}
	if err := g.Play(d, g.Cards(d)[0]); err == nil {
		t.Fatal("played before choosing")
	}
	if err := g.Choose(d, Contract{Kind: Trump, Suit: Hearts}); err != nil {
		t.Fatal(err)
	}
	if g.Turn() != d {
		t.Fatal("declarer does not lead")
	}
	if tl, nl := g.QuotaLeft(d); tl != 1 || nl != 3 {
		t.Fatalf("quota %d %d", tl, nl)
	}
}

func TestQuotasRestrictChoices(t *testing.T) {
	// Drive a game where everyone always picks the first legal contract, and
	// check the offer at each step against the quotas.
	g := NewGame(Deals(3))
	for !g.Done() {
		seat := g.Turn()
		if g.Phase() == Choosing {
			legal := g.LegalContracts(seat)
			tl, nl := g.QuotaLeft(seat)
			for _, c := range legal {
				if c.Kind == Trump && tl == 0 || c.Kind.IsNegative() && (nl == 0 || g.NegativeCount(c.Kind) >= MaxPerNegative) {
					t.Fatalf("hand %d offered %v (trump left %d, negative left %d)", g.HandNumber(), c, tl, nl)
				}
			}
			g.Choose(seat, legal[0])
			continue
		}
		g.Play(seat, Lowest(g.Legal(seat)))
	}
}
