package match

import (
	"context"
	"testing"

	"github.com/cenkalti/king-server/king"
	"github.com/cenkalti/king-server/protocol"
)

// The positions must be what their names claim. For card decisions, want
// lists the exact legal cards; "" means only check that a position exists.
var scenarioWant = map[string]string{
	"follow suit with a choice":                          "5S JS",
	"only one legal card":                                "4C",
	"no_queens: queen forced under a played king":        "QD",
	"no_kings_jacks: K or J forced under a played queen": "JC KC",
	"no_king_of_hearts: K♥ forced under a played A♥":     "KH",
	"no_king_of_hearts: void, must discard K♥":           "KH",
	"no_hearts: lead with only hearts left":              "3H 4H 5H 6H 7H 8H 9H TH JH QH KH AH",
	"last trick":                                         "",
}

func TestScenarios(t *testing.T) {
	sc := Scenarios()
	if len(sc) < 20 {
		t.Fatalf("only %d scenarios", len(sc))
	}
	for _, s := range sc {
		t.Run(s.Name, func(t *testing.T) {
			g, err := s.Game()
			if err != nil {
				t.Fatal(err)
			}
			req := protocol.NewRequest("v", g, 0, [4]string{"a", "b", "c", "d"})
			switch req.Kind {
			case protocol.PlayCard:
				if len(req.LegalCards) == 0 {
					t.Fatal("no legal cards")
				}
				if want, ok := scenarioWant[s.Name]; ok && want != "" {
					w := king.MustParseCards(want)
					king.SortHand(w)
					if len(w) != len(req.LegalCards) {
						t.Fatalf("legal %v want %v", req.LegalCards, w)
					}
					for i := range w {
						if w[i] != req.LegalCards[i] {
							t.Fatalf("legal %v want %v", req.LegalCards, w)
						}
					}
				}
			case protocol.ChooseContract:
				if len(req.LegalContracts) == 0 {
					t.Fatal("no legal contracts")
				}
			}
			// LowBot (= the fallback) always answers legally.
			resp, _ := LowBot{}.Decide(context.Background(), req)
			if v := Judge(req, resp, nil); v.Fault != "" {
				t.Fatalf("LowBot judged %s: %s", v.Fault, v.Detail)
			}
		})
	}
}

func TestScenarioSpecifics(t *testing.T) {
	get := func(name string) *protocol.Request {
		for _, s := range Scenarios() {
			if s.Name == name {
				g, err := s.Game()
				if err != nil {
					t.Fatal(err)
				}
				return protocol.NewRequest("v", g, 0, [4]string{"a", "b", "c", "d"})
			}
		}
		t.Fatalf("no scenario %q", name)
		return nil
	}
	if r := get("first hand: choose a contract (all 10 options)"); len(r.LegalContracts) != 10 {
		t.Fatalf("first hand offers %v", r.LegalContracts)
	}
	for _, c := range get("quota: trumps used up, only negatives offered").LegalContracts {
		if c.Kind == king.Trump {
			t.Fatalf("trump offered: %v", c)
		}
	}
	for _, c := range get("quota: negatives used up, only trumps offered").LegalContracts {
		if c.Kind != king.Trump {
			t.Fatalf("negative offered: %v", c)
		}
	}
	for _, c := range get("quota: a negative contract used twice is not offered").LegalContracts {
		if c.Kind == king.NoQueens {
			t.Fatal("no_queens offered a third time")
		}
	}
	if r := get("last trick"); len(r.State.Tricks) != 12 || len(r.LegalCards) != 1 {
		t.Fatalf("last trick: %d tricks, legal %v", len(r.State.Tricks), r.LegalCards)
	}
	if r := get("no_last_two: trick 12"); len(r.State.Tricks) != 11 {
		t.Fatalf("no_last_two: %d tricks", len(r.State.Tricks))
	}
	r := get("trump: void, must underruff a higher trump")
	for _, c := range r.LegalCards {
		if c.Suit != king.Spades {
			t.Fatalf("non-trump legal: %v", r.LegalCards)
		}
	}
	if !containsCard(r.LegalCards, "2S") {
		t.Fatalf("underruff 2S not legal: %v", r.LegalCards)
	}
	r = get("no_hearts: lead, hearts not yet allowed")
	for _, c := range r.LegalCards {
		if c.Suit == king.Hearts {
			t.Fatalf("heart lead offered: %v", r.LegalCards)
		}
	}
}

func containsCard(cs []king.Card, s string) bool {
	c, _ := king.ParseCard(s)
	for _, x := range cs {
		if x == c {
			return true
		}
	}
	return false
}
