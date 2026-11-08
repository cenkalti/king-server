package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/cenkalti/king-server/match"
)

// runPractice runs a match where only -bots seats are remote and house bots
// fill the rest.
func runPractice(args []string) error {
	fs := flag.NewFlagSet("practice", flag.ExitOnError)
	var mf matchFlags
	mf.register(fs)
	bots := fs.Int("bots", 1, "number of remote bots to wait for (0-4)")
	house := fs.String("house", "random,low,low", "house bot strategies for the empty seats, comma-separated (random or low); reused cyclically")
	fs.Parse(args)
	if *bots < 0 || *bots > 4 {
		return fmt.Errorf("-bots must be 0-4")
	}
	strategies := strings.Split(*house, ",")
	var players []match.Player
	var names []string
	for i := range 4 - *bots {
		s := strings.TrimSpace(strategies[i%len(strategies)])
		switch s {
		case "random":
			players = append(players, match.NewRandomBot(mf.seed+uint64(i)+1))
		case "low":
			players = append(players, match.LowBot{})
		default:
			return fmt.Errorf("unknown house strategy %q", s)
		}
		names = append(names, fmt.Sprintf("house-%s-%d", s, i+1))
	}
	return serveMatch(mf, *bots, players, names, "practice-")
}
