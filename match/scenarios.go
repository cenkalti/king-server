package match

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/cenkalti/king-server/king"
)

// Scenario is a fixed game position used by `king validate` to check that a
// bot answers tricky requests legally. Positions are reached by driving a
// real king.Game, so they can't drift from the rules.
type Scenario struct {
	Name  string
	build func() (*king.Game, error)
}

// Game builds the scenario's game, positioned at the decision to ask about.
func (s Scenario) Game() (*king.Game, error) { return s.build() }

// hand1 describes a position in the first hand. Seat 0 always holds the 2♦,
// so it declares and leads.
type hand1 struct {
	hold     [4]string // cards each seat must hold
	void     [4]string // suits each seat must not hold, e.g. "CH"
	contract king.Contract
	auto     int    // completed tricks to auto-play (lowest legal) first
	plays    string // then these cards, in turn order
}

func (h hand1) game() (*king.Game, error) {
	first, err := dealHand(h.hold, h.void)
	if err != nil {
		return nil, err
	}
	deals := king.Deals(1)
	deals[0] = first
	g := king.NewGame(deals)
	if err := g.Choose(0, h.contract); err != nil {
		return nil, err
	}
	for len(g.Hand().Tricks()) < h.auto {
		if err := g.Play(g.Turn(), king.Lowest(g.Legal(g.Turn()))); err != nil {
			return nil, err
		}
	}
	for _, c := range king.MustParseCards(h.plays) {
		if err := g.Play(g.Turn(), c); err != nil {
			return nil, fmt.Errorf("play %v by seat %d: %w", c, g.Turn(), err)
		}
	}
	return g, nil
}

// dealHand builds a full deal where each seat holds the given cards, has no
// cards of its void suits, and seat 0 has the 2♦.
func dealHand(hold, void [4]string) ([4][]king.Card, error) {
	var hands [4][]king.Card
	used := map[king.Card]bool{}
	hold[0] += " 2D"
	for s := range 4 {
		for _, c := range king.MustParseCards(hold[s]) {
			if used[c] {
				return hands, fmt.Errorf("card %v listed twice", c)
			}
			used[c] = true
			hands[s] = append(hands[s], c)
		}
	}
	voidIn := func(s int, c king.Card) bool { return strings.Contains(void[s], c.Suit.String()) }
	var rest []king.Card
	for _, c := range king.Deck() {
		if !used[c] {
			rest = append(rest, c)
		}
	}
	r := rand.New(rand.NewPCG(1, 1))
	r.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	// Place the most constrained cards first: fewest seats that can take them.
	open := func(c king.Card) int {
		n := 0
		for s := range 4 {
			if !voidIn(s, c) {
				n++
			}
		}
		return n
	}
	slices.SortStableFunc(rest, func(a, b king.Card) int { return open(a) - open(b) })
	for i, c := range rest {
		placed := false
		for k := range 4 {
			s := (i + k) % 4
			if len(hands[s]) < 13 && !voidIn(s, c) {
				hands[s] = append(hands[s], c)
				placed = true
				break
			}
		}
		if !placed {
			return hands, fmt.Errorf("cannot place %v with the given voids", c)
		}
	}
	for s := range 4 {
		if len(hands[s]) != 13 {
			return hands, fmt.Errorf("seat %d has %d cards", s, len(hands[s]))
		}
	}
	return hands, nil
}

// declare drives a game through whole hands: each hand the declarer chooses
// pick(g) and the hand is auto-played (lowest legal), until stop(g) is true
// at a contract choice.
func declare(pick func(*king.Game) king.Contract, stop func(*king.Game) bool) func() (*king.Game, error) {
	return func() (*king.Game, error) {
		g := king.NewGame(king.Deals(2))
		for !stop(g) {
			if g.Done() {
				return nil, fmt.Errorf("game ended before the scenario was reached")
			}
			if err := g.Choose(g.Turn(), pick(g)); err != nil {
				return nil, err
			}
			for g.Phase() == king.Playing {
				g.Play(g.Turn(), king.Lowest(g.Legal(g.Turn())))
			}
		}
		return g, nil
	}
}

var (
	trumpS = king.Contract{Kind: king.Trump, Suit: king.Spades}
	trumpH = king.Contract{Kind: king.Trump, Suit: king.Hearts}
)

func neg(k king.ContractKind) king.Contract { return king.Contract{Kind: k} }

// Scenarios returns every validation scenario.
func Scenarios() []Scenario {
	h := func(name string, s hand1) Scenario { return Scenario{name, s.game} }
	return []Scenario{
		{"first hand: choose a contract (all 10 options)", declare(nil, func(*king.Game) bool { return true })},

		h("lead: any card", hand1{contract: neg(king.NoTricks)}),
		h("follow suit with a choice", hand1{
			hold: [4]string{"7S", "5S JS 2H"}, void: [4]string{"", "S"}, contract: neg(king.NoTricks), plays: "7S"}),
		h("void in the led suit: discard anything", hand1{
			hold: [4]string{"3C"}, void: [4]string{"", "C"}, contract: neg(king.NoTricks), plays: "3C"}),
		h("only one legal card", hand1{
			hold: [4]string{"3C", "4C"}, void: [4]string{"", "C"}, contract: neg(king.NoTricks), plays: "3C"}),

		h("trump: void, must trump", hand1{
			hold: [4]string{"3H", "2S"}, void: [4]string{"", "H"}, contract: trumpS, plays: "3H"}),
		h("trump: void, must underruff a higher trump", hand1{
			hold: [4]string{"3H", "AS", "2S"}, void: [4]string{"", "H", "H"}, contract: trumpS, plays: "3H AS"}),
		h("trump: void with no trumps, discard anything", hand1{
			hold: [4]string{"3H"}, void: [4]string{"", "HS"}, contract: trumpS, plays: "3H"}),
		h("trump: lead, trumps allowed", hand1{hold: [4]string{"AS KS"}, contract: trumpS}),
		h("trump: hearts as trump", hand1{
			hold: [4]string{"3C", "KH"}, void: [4]string{"", "C"}, contract: trumpH, plays: "3C"}),

		h("no_queens: queen forced under a played king", hand1{
			hold: [4]string{"5D", "KD", "QD 3D"}, contract: neg(king.NoQueens), plays: "5D KD"}),
		h("no_kings_jacks: K or J forced under a played queen", hand1{
			hold: [4]string{"4C", "QC", "KC JC 2C"}, contract: neg(king.NoKingsJacks), plays: "4C QC"}),
		// Trick 1 breaks hearts (seat 1 discards 5♥) and seat 3 wins it with
		// A♦, then leads A♥ with seat 0 holding K♥ and 2♥.
		h("no_king_of_hearts: K♥ forced under a played A♥", hand1{
			hold: [4]string{"KH 2H", "5H", "3D", "AD AH"}, void: [4]string{"", "D"},
			contract: neg(king.NoKingOfHearts), plays: "2D 5H 3D AD AH"}),

		h("no_king_of_hearts: void, must discard K♥", hand1{
			hold: [4]string{"3C", "KH"}, void: [4]string{"", "C"}, contract: neg(king.NoKingOfHearts), plays: "3C"}),
		h("no_queens: void, must discard a queen", hand1{
			hold: [4]string{"3C", "QH QS"}, void: [4]string{"", "C"}, contract: neg(king.NoQueens), plays: "3C"}),
		h("no_kings_jacks: void, must discard a K or J", hand1{
			hold: [4]string{"3C", "KH JS"}, void: [4]string{"", "C"}, contract: neg(king.NoKingsJacks), plays: "3C"}),
		h("no_hearts: void, must discard a heart", hand1{
			hold: [4]string{"3C", "AH"}, void: [4]string{"", "C"}, contract: neg(king.NoHearts), plays: "3C"}),

		h("no_hearts: lead, hearts not yet allowed", hand1{hold: [4]string{"AH 2H"}, contract: neg(king.NoHearts)}),
		// Seat 1 holds A♦ and 12 hearts, wins trick 1 and must lead.
		h("no_hearts: lead with only hearts left", hand1{
			hold:     [4]string{"", "AD AH KH QH JH TH 9H 8H 7H 6H 5H 4H 3H", "3D", "4D"},
			contract: neg(king.NoHearts), plays: "2D AD 3D 4D"}),

		h("last trick", hand1{contract: neg(king.NoTricks), auto: 12}),
		h("no_last_two: trick 12", hand1{contract: neg(king.NoLastTwo), auto: 11}),

		{"quota: trumps used up, only negatives offered", declare(
			func(g *king.Game) king.Contract {
				if t, _ := g.QuotaLeft(g.Turn()); g.Turn() == g.FirstDeclarer() && t > 0 {
					return trumpS
				}
				return g.LegalContracts(g.Turn())[0]
			},
			func(g *king.Game) bool {
				t, _ := g.QuotaLeft(g.FirstDeclarer())
				return t == 0 && g.Turn() == g.FirstDeclarer() && g.Phase() == king.Choosing
			},
		)},
		{"quota: negatives used up, only trumps offered", declare(
			func(g *king.Game) king.Contract {
				legal := g.LegalContracts(g.Turn())
				if g.Turn() != g.FirstDeclarer() {
					if i := slices.IndexFunc(legal, func(c king.Contract) bool { return c.Kind == king.Trump }); i >= 0 {
						return legal[i]
					}
				}
				return legal[0] // negatives come first in the order
			},
			func(g *king.Game) bool {
				_, n := g.QuotaLeft(g.FirstDeclarer())
				return n == 0 && g.Turn() == g.FirstDeclarer() && g.Phase() == king.Choosing
			},
		)},
		{"quota: a negative contract used twice is not offered", declare(
			func(*king.Game) king.Contract { return neg(king.NoQueens) },
			func(g *king.Game) bool { return g.NegativeCount(king.NoQueens) == 2 && g.Phase() == king.Choosing },
		)},
	}
}
