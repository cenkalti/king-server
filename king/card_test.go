package king

import (
	"encoding/json"
	"testing"
)

func TestDeck(t *testing.T) {
	d := Deck()
	if len(d) != 52 {
		t.Fatalf("deck has %d cards", len(d))
	}
	seen := map[Card]bool{}
	for _, c := range d {
		if !c.Valid() || seen[c] {
			t.Fatalf("bad or duplicate card %v", c)
		}
		seen[c] = true
	}
}

func TestDealDeterministic(t *testing.T) {
	a, b := Deal(42), Deal(42)
	for s := range a {
		if len(a[s]) != 13 {
			t.Fatalf("seat %d got %d cards", s, len(a[s]))
		}
		for i := range a[s] {
			if a[s][i] != b[s][i] {
				t.Fatalf("seed 42 dealt differently")
			}
		}
	}
	seen := map[Card]bool{}
	for _, h := range a {
		for _, c := range h {
			if seen[c] {
				t.Fatalf("duplicate %v", c)
			}
			seen[c] = true
		}
	}
	if len(seen) != 52 {
		t.Fatalf("dealt %d distinct cards", len(seen))
	}
	if c := Deal(43); c[0][0] == a[0][0] && c[1][0] == a[1][0] && c[2][0] == a[2][0] {
		t.Fatalf("different seeds look identical")
	}
}

func TestLowest(t *testing.T) {
	cs := []Card{{5, Spades}, {5, Clubs}, {9, Hearts}}
	if got := Lowest(cs); got != (Card{5, Clubs}) {
		t.Fatalf("Lowest = %v", got)
	}
}

func TestCardJSON(t *testing.T) {
	c := Card{Rank: Queen, Suit: Spades}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"rank":12,"suit":"S"}` {
		t.Fatalf("got %s", b)
	}
	var back Card
	if err := json.Unmarshal(b, &back); err != nil || back != c {
		t.Fatalf("round trip: %v %v", back, err)
	}
	if err := json.Unmarshal([]byte(`{"rank":3,"suit":"X"}`), &back); err == nil {
		t.Fatal("accepted bad suit")
	}
}

func TestCardString(t *testing.T) {
	for c, want := range map[Card]string{{10, Hearts}: "TH", {Queen, Spades}: "QS", {2, Clubs}: "2C", {Ace, Diamonds}: "AD"} {
		if c.String() != want {
			t.Errorf("%#v: %s want %s", c, c, want)
		}
	}
}

func TestParseCard(t *testing.T) {
	for _, c := range Deck() {
		got, err := ParseCard(c.String())
		if err != nil || got != c {
			t.Fatalf("%v: %v %v", c, got, err)
		}
	}
	for _, bad := range []string{"", "1S", "QX", "10H", "QSS"} {
		if _, err := ParseCard(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
