package match

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/king-server/king"
	"github.com/cenkalti/king-server/protocol"
)

var names = [4]string{"alice", "bob", "cenk", "deniz"}

func houseBots() [4]Player {
	return [4]Player{NewRandomBot(1), NewRandomBot(2), LowBot{}, NewRandomBot(3)}
}

// runMatch runs a match and returns the standings, the error and the log
// lines.
func runMatch(t *testing.T, r *Runner, ctx context.Context) (*Standings, error, []map[string]any, string) {
	t.Helper()
	dir := t.TempDir()
	l, err := NewLogger(filepath.Join(dir, "log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	r.Log = l
	if r.Names == ([4]string{}) {
		r.Names = names
	}
	if r.Timeout == 0 {
		r.Timeout = time.Second
	}
	st, runErr := r.Run(ctx)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := st.WriteResults(dir); err != nil {
		t.Fatal(err)
	}
	return st, runErr, readLog(t, filepath.Join(dir, "log.jsonl")), dir
}

func readLog(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var v map[string]any
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			t.Fatalf("invalid log line %q: %v", sc.Text(), err)
		}
		out = append(out, v)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func records(log []map[string]any, typ string) []map[string]any {
	var out []map[string]any
	for _, v := range log {
		if v["type"] == typ {
			out = append(out, v)
		}
	}
	return out
}

func TestFullMatchDeterministic(t *testing.T) {
	var dirs [2]string
	for i := range dirs {
		r := &Runner{Players: houseBots(), Seed: 42, Games: 100}
		st, err, log, dir := runMatch(t, r, context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if st.Played() != 100 || len(records(log, "game_end")) != 100 {
			t.Fatalf("played %d", st.Played())
		}
		if len(records(log, "hand_end")) != 100*king.HandsPerGame {
			t.Fatalf("hand_end records: %d", len(records(log, "hand_end")))
		}
		for _, row := range st.Table() {
			if row.Strikes != 0 {
				t.Fatalf("house bot %s has strikes", row.Name)
			}
		}
		dirs[i] = dir
	}
	for _, f := range []string{"log.jsonl", "results.json"} {
		a, _ := os.ReadFile(filepath.Join(dirs[0], f))
		b, _ := os.ReadFile(filepath.Join(dirs[1], f))
		if !bytes.Equal(a, b) {
			t.Fatalf("%s differs between runs with the same seed", f)
		}
	}
}

func TestDuplicateSeating(t *testing.T) {
	r := &Runner{Players: houseBots(), Seed: 7, Games: 8}
	_, err, log, _ := runMatch(t, r, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seatsBySet := map[float64]map[string][]int{}
	for _, v := range records(log, "game_start") {
		set := v["deal_set"].(float64)
		if seatsBySet[set] == nil {
			seatsBySet[set] = map[string][]int{}
		}
		for seat, name := range v["seats"].([]any) {
			seatsBySet[set][name.(string)] = append(seatsBySet[set][name.(string)], seat)
		}
	}
	if len(seatsBySet) != 2 {
		t.Fatalf("deal sets: %v", seatsBySet)
	}
	for set, bySeat := range seatsBySet {
		for name, seats := range bySeat {
			slices.Sort(seats)
			if !slices.Equal(seats, []int{0, 1, 2, 3}) {
				t.Fatalf("set %v: %s sat in %v", set, name, seats)
			}
		}
	}
	// Same deal set → same first declarer seat (the 2♦ moves with the seat).
	first := map[float64]float64{}
	for _, v := range records(log, "decision") {
		if v["hand"].(float64) == 1 && v["kind"] == "choose_contract" {
			set := float64(int(v["game"].(float64)) / 4)
			if s, ok := first[set]; ok && s != v["seat"].(float64) {
				t.Fatalf("set %v: first declarer seat differs between rotations", set)
			}
			first[set] = v["seat"].(float64)
		}
	}
}

// faulty misbehaves for its first n calls, then plays like LowBot.
type faulty struct {
	n     int32
	calls atomic.Int32
	mode  string // "slow", "illegal", "disconnect"
	waits atomic.Int32
}

func (f *faulty) Decide(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
	if f.calls.Add(1) <= f.n {
		switch f.mode {
		case "slow":
			<-ctx.Done()
			return nil, ctx.Err()
		case "illegal":
			bad := king.Card{Rank: 99, Suit: king.Spades}
			return answer(req, &bad, &king.Contract{Kind: king.Trump, Suit: 9}), nil
		case "disconnect":
			return nil, ErrDisconnected
		}
	}
	return LowBot{}.Decide(ctx, req)
}

func (f *faulty) WaitConnected(ctx context.Context) error {
	f.waits.Add(1)
	time.Sleep(time.Millisecond)
	return ctx.Err()
}

func TestFaults(t *testing.T) {
	for _, mode := range []string{"slow", "illegal"} {
		t.Run(mode, func(t *testing.T) {
			bad := &faulty{n: 3, mode: mode}
			r := &Runner{Players: [4]Player{LowBot{}, bad, LowBot{}, LowBot{}}, Seed: 1, Games: 1, Timeout: 20 * time.Millisecond}
			st, err, log, _ := runMatch(t, r, context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got := st.Table(); got[slices.IndexFunc(got, func(s protocol.Standing) bool { return s.Name == "bob" })].Strikes != 3 {
				t.Fatalf("strikes: %+v", got)
			}
			want := FaultTimeout
			if mode == "illegal" {
				want = FaultIllegal
			}
			n := 0
			for _, v := range records(log, "decision") {
				if v["fault"] == nil {
					continue
				}
				n++
				if v["fault"] != want || v["bot"] != "bob" {
					t.Fatalf("fault record %v", v)
				}
				// The fallback is the lowest legal option.
				b, _ := json.Marshal(v)
				var d DecisionRecord
				json.Unmarshal(b, &d)
				if d.Kind == protocol.PlayCard && *d.Card != king.Lowest(d.LegalCards) {
					t.Fatalf("fallback card %v, lowest %v", d.Card, king.Lowest(d.LegalCards))
				}
				if d.Kind == protocol.ChooseContract && *d.Contract != d.LegalContracts[0] {
					t.Fatalf("fallback contract %v", d.Contract)
				}
			}
			if n != 3 {
				t.Fatalf("%d fault records", n)
			}
		})
	}
}

func TestDisconnectPauses(t *testing.T) {
	bad := &faulty{n: 2, mode: "disconnect"}
	var pauses []bool
	r := &Runner{
		Players: [4]Player{LowBot{}, LowBot{}, bad, LowBot{}}, Seed: 1, Games: 1,
		OnPause: func(_ string, p bool) { pauses = append(pauses, p) },
	}
	st, err, log, _ := runMatch(t, r, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range st.Table() {
		if row.Strikes != 0 {
			t.Fatalf("disconnect caused a strike: %+v", row)
		}
	}
	if bad.waits.Load() != 2 || len(records(log, "pause")) != 2 || len(records(log, "resume")) != 2 {
		t.Fatalf("waits=%d log pauses=%d", bad.waits.Load(), len(records(log, "pause")))
	}
	if !slices.Equal(pauses, []bool{true, false, true, false}) {
		t.Fatalf("OnPause calls %v", pauses)
	}
}

func TestCancelWritesPartialResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runner{Players: houseBots(), Seed: 3, Games: 10}
	r.OnGameEnd = func(res GameResult, _ *Standings) {
		if res.Game == 1 {
			cancel()
		}
	}
	st, err, log, dir := runMatch(t, r, ctx)
	if err == nil {
		t.Fatal("expected an error after cancel")
	}
	if st.Played() != 2 {
		t.Fatalf("played %d", st.Played())
	}
	if end := records(log, "match_end"); len(end) != 1 || end[0]["complete"] != false {
		t.Fatalf("match_end %v", end)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "results.json"))
	var f resultsFile
	if err := json.Unmarshal(b, &f); err != nil || f.Complete || f.Played != 2 {
		t.Fatalf("results %+v %v", f, err)
	}
}

// hashBot picks a legal option from a hash of the request id, so its play
// doesn't depend on the order requests arrive in.
type hashBot struct{ salt uint64 }

func (b hashBot) Decide(_ context.Context, req *protocol.Request) (*protocol.Response, error) {
	h := b.salt
	for _, c := range req.ID {
		h = h*1099511628211 ^ uint64(c)
	}
	if req.Kind == protocol.ChooseContract {
		c := req.LegalContracts[h%uint64(len(req.LegalContracts))]
		return answer(req, nil, &c), nil
	}
	c := req.LegalCards[h%uint64(len(req.LegalCards))]
	return answer(req, &c, nil), nil
}

func TestParallelMatchesSequential(t *testing.T) {
	players := func() [4]Player { return [4]Player{hashBot{1}, hashBot{2}, LowBot{}, hashBot{3}} }
	seq, err, _, _ := runMatch(t, &Runner{Players: players(), Seed: 9, Games: 40}, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	par, err, log, _ := runMatch(t, &Runner{Players: players(), Seed: 9, Games: 40, Parallel: 8}, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(seq.Table(), par.Table()) || !slices.Equal(seq.Games(), par.Games()) {
		t.Fatalf("parallel standings differ:\n%s\n%s", seq.FormatTable(), par.FormatTable())
	}
	if len(records(log, "game_end")) != 40 {
		t.Fatalf("game_end records: %d", len(records(log, "game_end")))
	}
}

func TestParallelCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runner{Players: houseBots(), Seed: 3, Games: 100, Parallel: 4}
	r.OnGameEnd = func(_ GameResult, st *Standings) {
		if st.Played() == 5 {
			cancel()
		}
	}
	st, err, log, _ := runMatch(t, r, ctx)
	if err == nil || st.Played() >= 100 {
		t.Fatalf("err=%v played=%d", err, st.Played())
	}
	if end := records(log, "match_end"); len(end) != 1 || end[0]["complete"] != false {
		t.Fatalf("match_end %v", end)
	}
}
