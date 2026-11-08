// Command king runs King bot matches.
//
//	king serve     run a match between 4 connected bots
//	king practice  run a match with house bots filling empty seats
//	king validate  check one bot against edge-case requests
//	king schema    print the protocol JSON Schema
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/cenkalti/king-server/match"
	"github.com/cenkalti/king-server/schema"
	"github.com/cenkalti/king-server/server"
)

const usage = `usage: king <command> [flags]

commands:
  serve     run a match between 4 connected bots
  practice  run a match with house bots filling empty seats
  validate  check one bot against edge-case requests
  schema    print the protocol JSON Schema

Run "king <command> -h" for a command's flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "serve":
		err = runServe(args)
	case "practice":
		err = runPractice(args)
	case "validate":
		err = runValidate(args)
	case "schema":
		_, err = os.Stdout.Write(schema.JSON)
	case "-h", "-help", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "king:", err)
		os.Exit(1)
	}
}

// matchFlags are shared by serve and practice.
type matchFlags struct {
	addr     string
	games    int
	seed     uint64
	timeout  time.Duration
	out      string
	parallel int
}

func (m *matchFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&m.addr, "addr", "127.0.0.1:8080", "listen address")
	fs.IntVar(&m.games, "games", 100, "number of games (multiples of 4 give complete seat rotations)")
	fs.Uint64Var(&m.seed, "seed", 0, "match seed; 0 picks a random one")
	fs.DurationVar(&m.timeout, "timeout", 2*time.Second, "time a bot has for one decision")
	fs.StringVar(&m.out, "out", "matches", "directory for match results")
	fs.IntVar(&m.parallel, "parallel", 1, "games to run at once; bots then get concurrent requests")
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var mf matchFlags
	mf.register(fs)
	fs.Parse(args)
	return serveMatch(mf, 4, nil, nil, "")
}

// serveMatch starts the hub, waits for `remote` bots, fills the remaining
// seats with house bots and runs the match.
// Results go to <out>/<dirPrefix><timestamp>/.
func serveMatch(mf matchFlags, remote int, house []match.Player, houseNames []string, dirPrefix string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if mf.seed == 0 {
		mf.seed = rand.Uint64()
	}
	hub := server.NewHub(remote)
	hub.Logf = logf
	if remote > 0 {
		ln, err := net.Listen("tcp", mf.addr)
		if err != nil {
			return err
		}
		mux := http.NewServeMux()
		mux.Handle("/ws", hub)
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "King server. Bots connect with WebSocket to /ws.")
		})
		srv := &http.Server{Handler: mux}
		go srv.Serve(ln)
		defer srv.Close()

		logf("listening on ws://%s/ws", ln.Addr())
		logf("for remote bots: cloudflared tunnel --url http://%s  → bots connect to wss://<host>.trycloudflare.com/ws", ln.Addr())
		logf("waiting for %d bot(s)...", remote)
		select {
		case <-hub.Ready():
		case <-ctx.Done():
			return errors.New("interrupted while waiting for bots")
		}
	}
	remotes := hub.Start()

	var r match.Runner
	for i, p := range remotes {
		r.Names[i], r.Players[i] = p.Name(), p
	}
	for i := range house {
		r.Names[len(remotes)+i], r.Players[len(remotes)+i] = houseNames[i], house[i]
	}
	logf("seats (rotation 0): %v", r.Names)
	logf("seed %d, %d games, timeout %s, parallel %d", mf.seed, mf.games, mf.timeout, max(mf.parallel, 1))

	dir := filepath.Join(mf.out, dirPrefix+time.Now().Format("2006-01-02_15-04-05"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	log, err := match.NewLogger(filepath.Join(dir, "log.jsonl"))
	if err != nil {
		return err
	}
	r.Seed, r.Games, r.Timeout, r.Log, r.Parallel = mf.seed, mf.games, mf.timeout, log, mf.parallel
	start := time.Now()
	r.OnGameEnd = func(res match.GameResult, st *match.Standings) {
		top := st.Table()[0]
		logf("game %d/%d done (%s)  leader: %s %.1f", st.Played(), mf.games, time.Since(start).Round(time.Second), top.Name, top.PlacementPoints)
	}
	r.OnPause = func(bot string, paused bool) {
		if paused {
			logf("PAUSED: waiting for %s to reconnect", bot)
		} else {
			logf("resumed: %s is back", bot)
		}
	}

	st, runErr := r.Run(ctx)
	if err := log.Close(); err != nil && runErr == nil {
		runErr = err
	}
	if err := st.WriteResults(dir); err != nil {
		return err
	}
	for _, p := range remotes {
		p.SendMatchEnd(st.Table())
	}
	fmt.Println()
	fmt.Print(st.FormatTable())
	fmt.Printf("\nresults in %s (%s)\n", dir, time.Since(start).Round(time.Second))
	if errors.Is(runErr, context.Canceled) {
		return errors.New("interrupted; partial results written")
	}
	return runErr
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s  %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}
