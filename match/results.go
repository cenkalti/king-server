// Package match runs a series of King games between four players and keeps
// the standings.
package match

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cenkalti/king-server/protocol"
)

// placementPoints are awarded for 1st..4th place in a game.
var placementPoints = [4]float64{3, 2, 1, 0}

// Placement returns each bot's placement points for one game, given the
// bots' game scores. Tied bots split the points of the positions they share.
func Placement(scores [4]int) [4]float64 {
	var out [4]float64
	for i := range scores {
		higher, tied := 0, 0
		for j := range scores {
			switch {
			case scores[j] > scores[i]:
				higher++
			case scores[j] == scores[i]:
				tied++
			}
		}
		sum := 0.0
		for p := higher; p < higher+tied; p++ {
			sum += placementPoints[p]
		}
		out[i] = sum / float64(tied)
	}
	return out
}

// wins returns each bot's share of the game win: 1 for a sole winner, 1/k for
// each of k bots tied for first.
func wins(scores [4]int) [4]float64 {
	best := slices.Max(scores[:])
	n := 0
	for _, s := range scores {
		if s == best {
			n++
		}
	}
	var out [4]float64
	for i, s := range scores {
		if s == best {
			out[i] = 1 / float64(n)
		}
	}
	return out
}

// GameResult is one finished game, indexed by bot (not seat).
type GameResult struct {
	Game      int        `json:"game"`
	Scores    [4]int     `json:"scores"`
	Placement [4]float64 `json:"placement"`
}

// Standings accumulates results by bot index 0..3.
type Standings struct {
	Names   [4]string
	Seed    uint64
	Planned int

	games     []GameResult
	placement [4]float64
	total     [4]int
	wins      [4]float64
	strikes   [4]int
}

func NewStandings(names [4]string, seed uint64, planned int) *Standings {
	return &Standings{Names: names, Seed: seed, Planned: planned}
}

// AddGame records a finished game. scores are indexed by bot.
func (s *Standings) AddGame(game int, scores [4]int) GameResult {
	r := GameResult{Game: game, Scores: scores, Placement: Placement(scores)}
	w := wins(scores)
	for b := range 4 {
		s.placement[b] += r.Placement[b]
		s.total[b] += scores[b]
		s.wins[b] += w[b]
	}
	s.games = append(s.games, r)
	return r
}

// AddStrike records a fault by bot.
func (s *Standings) AddStrike(bot int) { s.strikes[bot]++ }

// Played is the number of finished games.
func (s *Standings) Played() int { return len(s.games) }

// Games returns every finished game, in the order they were added.
func (s *Standings) Games() []GameResult {
	return slices.SortedFunc(slices.Values(s.games), func(a, b GameResult) int { return a.Game - b.Game })
}

// Table returns the standings sorted by placement points, then total points,
// then name. Bots equal on both points share a rank.
func (s *Standings) Table() []protocol.Standing {
	rows := make([]protocol.Standing, 4)
	for b := range 4 {
		rows[b] = protocol.Standing{
			Name:            s.Names[b],
			PlacementPoints: s.placement[b],
			TotalPoints:     s.total[b],
			GamesWon:        s.wins[b],
			Strikes:         s.strikes[b],
		}
	}
	slices.SortFunc(rows, func(a, b protocol.Standing) int {
		return cmp.Or(
			cmp.Compare(b.PlacementPoints, a.PlacementPoints),
			cmp.Compare(b.TotalPoints, a.TotalPoints),
			cmp.Compare(a.Name, b.Name),
		)
	})
	for i := range rows {
		rows[i].Rank = i + 1
		if i > 0 && rows[i].PlacementPoints == rows[i-1].PlacementPoints && rows[i].TotalPoints == rows[i-1].TotalPoints {
			rows[i].Rank = rows[i-1].Rank
		}
	}
	return rows
}

// FormatTable renders the standings as a plain-text table.
func (s *Standings) FormatTable() string {
	var b strings.Builder
	status := ""
	if s.Played() < s.Planned {
		status = " (incomplete)"
	}
	fmt.Fprintf(&b, "Games: %d/%d%s  Seed: %d\n\n", s.Played(), s.Planned, status, s.Seed)
	fmt.Fprintf(&b, "%-4s  %-16s  %9s  %8s  %7s  %7s\n", "Rank", "Bot", "Placement", "Points", "Wins", "Strikes")
	for _, r := range s.Table() {
		fmt.Fprintf(&b, "%-4d  %-16s  %9.1f  %8d  %7s  %7d\n", r.Rank, r.Name, r.PlacementPoints, r.TotalPoints, formatWins(r.GamesWon), r.Strikes)
	}
	return b.String()
}

func formatWins(w float64) string {
	if w == float64(int(w)) {
		return fmt.Sprint(int(w))
	}
	return fmt.Sprintf("%.2f", w)
}

type resultsFile struct {
	Seed      uint64              `json:"seed"`
	Planned   int                 `json:"games_planned"`
	Played    int                 `json:"games_played"`
	Complete  bool                `json:"complete"`
	Standings []protocol.Standing `json:"standings"`
	Games     []GameResult        `json:"games"`
	Bots      [4]string           `json:"bots"`
}

// WriteResults writes results.json and standings.txt into dir.
func (s *Standings) WriteResults(dir string) error {
	f := resultsFile{
		Seed:      s.Seed,
		Planned:   s.Planned,
		Played:    s.Played(),
		Complete:  s.Played() == s.Planned,
		Standings: s.Table(),
		Games:     s.Games(),
		Bots:      s.Names,
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "results.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "standings.txt"), []byte(s.FormatTable()), 0o644)
}
