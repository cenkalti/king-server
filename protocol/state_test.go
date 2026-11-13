package protocol

import (
	"encoding/json"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/cenkalti/king-server/king"
)

var players = [4]string{"a", "b", "c", "d"}

// Every snapshot shows exactly the seat's own cards and never a card still
// held by another seat.
func TestSnapshotHidesOtherHands(t *testing.T) {
	r := rand.New(rand.NewPCG(9, 9))
	for range 50 {
		g := king.NewGame(king.Deals(r.Uint64()))
		for n := 0; !g.Done(); n++ {
			seat := g.Turn()
			req := NewRequest("x", g, 1, players)
			b, err := json.Marshal(req)
			if err != nil {
				t.Fatal(err)
			}
			var back Request
			if err := json.Unmarshal(b, &back); err != nil {
				t.Fatal(err)
			}
			st := back.State
			if st.Seat != seat || !slices.Equal(st.Cards, g.Cards(seat)) {
				t.Fatalf("seat %d: wrong own cards", seat)
			}
			visible := slices.Clone(st.Cards)
			for _, tr := range append(st.Tricks, st.CurrentTrick) {
				visible = append(visible, tr.Cards...)
			}
			for other := range 4 {
				if other == seat {
					continue
				}
				for _, c := range g.Cards(other) {
					if slices.Contains(visible, c) {
						t.Fatalf("seat %d sees %v held by seat %d", seat, c, other)
					}
				}
			}
			switch back.Kind {
			case ChooseContract:
				if len(back.LegalContracts) == 0 || back.LegalCards != nil || st.Contract != nil {
					t.Fatalf("bad choose_contract request: %s", b)
				}
				g.Choose(seat, back.LegalContracts[r.IntN(len(back.LegalContracts))])
			case PlayCard:
				if len(back.LegalCards) == 0 || back.LegalContracts != nil || st.Contract == nil {
					t.Fatalf("bad play_card request: %s", b)
				}
				g.Play(seat, back.LegalCards[r.IntN(len(back.LegalCards))])
			}
		}
	}
}

func TestSnapshotHistoryAndQuotas(t *testing.T) {
	g := king.NewGame(king.Deals(5))
	d := g.Declarer()
	g.Choose(d, king.Contract{Kind: king.NoKingOfHearts})
	for g.HandNumber() == 1 {
		g.Play(g.Turn(), g.Legal(g.Turn())[0])
	}
	st := Snapshot(g, 3, 0, players)
	if len(st.History) != 1 || st.History[0].Contract.Kind != king.NoKingOfHearts || st.History[0].Declarer != d {
		t.Fatalf("history %+v", st.History)
	}
	if st.NegativeCounts["no_king_of_hearts"] != 1 || st.Quotas[d] != (Quota{Trump: 2, Negative: 2}) {
		t.Fatalf("counts %v quotas %v", st.NegativeCounts, st.Quotas)
	}
	if st.Contract != nil || st.HandNumber != 2 || st.GameID != 3 {
		t.Fatalf("state %+v", st)
	}
	sum := 0
	for _, v := range st.Totals {
		sum += v
	}
	if sum != -320 {
		t.Fatalf("totals %v", st.Totals)
	}
}

// Arrays are always JSON arrays, never null, in every snapshot (the schema
// requires arrays). The first trick of a hand used to send "tricks": null.
func TestSnapshotArraysNeverNull(t *testing.T) {
	g := king.NewGame(king.Deals(3))
	check := func() {
		for seat := range 4 {
			b, _ := json.Marshal(Snapshot(g, 0, seat, players))
			for _, f := range []string{`"tricks":null`, `"cards":null`, `"history":null`} {
				if strings.Contains(string(b), f) {
					t.Fatalf("hand %d seat %d: %s in %s", g.HandNumber(), seat, f, b)
				}
			}
		}
	}
	for !g.Done() {
		check()
		if g.Phase() == king.Choosing {
			g.Choose(g.Turn(), g.LegalContracts(g.Turn())[0])
		} else {
			g.Play(g.Turn(), g.Legal(g.Turn())[0])
		}
	}
	check()
}
