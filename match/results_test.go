package match

import (
	"bufio"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestPlacement(t *testing.T) {
	tests := []struct {
		scores [4]int
		want   [4]float64
	}{
		{[4]int{100, 50, 0, -150}, [4]float64{3, 2, 1, 0}},
		{[4]int{-150, 0, 50, 100}, [4]float64{0, 1, 2, 3}},
		{[4]int{100, 100, 0, -200}, [4]float64{2.5, 2.5, 1, 0}},    // two-way tie for 1st
		{[4]int{300, -100, -100, -100}, [4]float64{3, 1, 1, 1}},    // three-way tie for 2nd
		{[4]int{0, 0, 0, 0}, [4]float64{1.5, 1.5, 1.5, 1.5}},       // four-way tie
		{[4]int{200, 0, -100, -100}, [4]float64{3, 2, 0.5, 0.5}},   // tie for 3rd
		{[4]int{100, 100, 100, -300}, [4]float64{2, 2, 2, 0}},      // three-way tie for 1st
		{[4]int{-300, 100, -300, 500}, [4]float64{0.5, 2, 0.5, 3}}, // tie for last
	}
	for _, tt := range tests {
		got := Placement(tt.scores)
		if got != tt.want {
			t.Errorf("Placement(%v) = %v, want %v", tt.scores, got, tt.want)
		}
		if sum := got[0] + got[1] + got[2] + got[3]; sum != 6 {
			t.Errorf("Placement(%v) sums to %v", tt.scores, sum)
		}
	}
}

func sampleStandings() *Standings {
	s := NewStandings([4]string{"alice", "bob", "cenk", "deniz"}, 42, 4)
	s.AddGame(0, [4]int{500, -100, -200, -200})
	s.AddGame(1, [4]int{100, 100, -50, -150})
	s.AddGame(2, [4]int{-300, 400, 0, -100})
	s.AddStrike(3)
	s.AddStrike(3)
	return s
}

func TestStandingsTable(t *testing.T) {
	rows := sampleStandings().Table()
	// bob: 2 + 2.5 + 3 = 7.5; alice: 3 + 2.5 + 0 = 5.5.
	if rows[0].Name != "bob" || rows[0].PlacementPoints != 7.5 || rows[0].GamesWon != 1.5 {
		t.Fatalf("first row %+v", rows[0])
	}
	if rows[1].Name != "alice" || rows[1].PlacementPoints != 5.5 || rows[1].GamesWon != 1.5 {
		t.Fatalf("second row %+v", rows[1])
	}
	if rows[3].Name != "deniz" || rows[3].Strikes != 2 {
		t.Fatalf("last row %+v", rows[3])
	}
}

func TestStandingsGolden(t *testing.T) {
	got := sampleStandings().FormatTable()
	path := filepath.Join("testdata", "standings.golden")
	if *update {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(path, []byte(got), 0o644)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("standings table changed (run with -update):\n%s\nwant:\n%s", got, want)
	}
}

func TestWriteResults(t *testing.T) {
	dir := t.TempDir()
	if err := sampleStandings().WriteResults(dir); err != nil {
		t.Fatal(err)
	}
	var f resultsFile
	b, _ := os.ReadFile(filepath.Join(dir, "results.json"))
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	if f.Complete || f.Played != 3 || f.Seed != 42 || len(f.Games) != 3 {
		t.Fatalf("results %+v", f)
	}
}

func TestLoggerJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	l, err := NewLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() { l.Log(map[string]any{"i": i, "s": "x\ny"}) })
	}
	wg.Wait()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(path)
	defer f.Close()
	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		var v map[string]any
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			t.Fatalf("line %d: %v", n, err)
		}
		n++
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if n != 50 {
		t.Fatalf("%d lines", n)
	}
}
