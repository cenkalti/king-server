package match

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/cenkalti/king-server/king"
	"github.com/cenkalti/king-server/protocol"
)

// Runner plays a match of Games games between four players.
//
// Games are grouped into deal sets of four. All games in a deal set use the
// same 20 deals; in rotation r (0..3) bot i sits in seat (i+r)%4, so every
// bot plays every seat's cards against the same opponents.
type Runner struct {
	Names   [4]string
	Players [4]Player
	Seed    uint64
	Games   int
	// Timeout is how long a connected player has for one decision.
	Timeout time.Duration
	Log     *Logger
	// Parallel is how many games run at once. 0 or 1 runs them one by one.
	// Bots then get requests from several games concurrently.
	Parallel int

	// OnGameEnd, if set, is called after every game.
	OnGameEnd func(GameResult, *Standings)
	// OnPause, if set, is called when the match pauses for a disconnected
	// bot (paused=true) and when it resumes.
	OnPause func(bot string, paused bool)

	mu sync.Mutex // guards the standings and OnGameEnd in parallel runs
}

// Fault reasons recorded in the log.
const (
	FaultTimeout = "timeout"
	FaultIllegal = "illegal"
	FaultError   = "error"
)

// Log records. Every record has a "type" field.
type (
	GameStartRecord struct {
		Type     string    `json:"type"`
		Game     int       `json:"game"`
		DealSet  int       `json:"deal_set"`
		Rotation int       `json:"rotation"`
		Seats    [4]string `json:"seats"`
	}
	DecisionRecord struct {
		Type           string          `json:"type"`
		Game           int             `json:"game"`
		Hand           int             `json:"hand"`
		Seat           int             `json:"seat"`
		Bot            string          `json:"bot"`
		Kind           protocol.Kind   `json:"kind"`
		LegalCards     []king.Card     `json:"legal_cards,omitempty"`
		LegalContracts []king.Contract `json:"legal_contracts,omitempty"`
		Card           *king.Card      `json:"card,omitempty"`
		Contract       *king.Contract  `json:"contract,omitempty"`
		Fault          string          `json:"fault,omitempty"`
		Detail         string          `json:"detail,omitempty"`
	}
	HandEndRecord struct {
		Type     string        `json:"type"`
		Game     int           `json:"game"`
		Hand     int           `json:"hand"`
		Declarer int           `json:"declarer"`
		Contract king.Contract `json:"contract"`
		Scores   [4]int        `json:"scores"` // by seat
	}
	GameEndRecord struct {
		Type string `json:"type"`
		GameResult
	}
	PauseRecord struct {
		Type string `json:"type"` // "pause" or "resume"
		Game int    `json:"game"`
		Bot  string `json:"bot"`
	}
	MatchEndRecord struct {
		Type      string              `json:"type"`
		Complete  bool                `json:"complete"`
		Standings []protocol.Standing `json:"standings"`
	}
)

// DealSetSeeds derives one seed per deal set from the match seed.
func DealSetSeeds(seed uint64, sets int) []uint64 {
	r := rand.New(rand.NewPCG(seed, 0x6b696e67)) // "king"
	out := make([]uint64, sets)
	for i := range out {
		out[i] = r.Uint64()
	}
	return out
}

// SeatOf returns the seat of bot in the given rotation.
func SeatOf(bot, rotation int) int { return (bot + rotation) % 4 }

// BotAt returns the bot sitting in seat in the given rotation.
func BotAt(seat, rotation int) int { return (seat - rotation%4 + 4) % 4 }

// Run plays the match. It always returns the standings so far; the error is
// non-nil if ctx was cancelled or a player failed in a way the runner cannot
// recover from.
func (r *Runner) Run(ctx context.Context) (*Standings, error) {
	st := NewStandings(r.Names, r.Seed, r.Games)
	seeds := DealSetSeeds(r.Seed, (r.Games+3)/4)
	var err error
	if r.Parallel <= 1 {
		for g := 0; g < r.Games && err == nil; g++ {
			var res GameResult
			if res, err = r.playGame(ctx, g, seeds[g/4], st); err == nil {
				r.finishGame(g, res, st)
			}
		}
	} else {
		err = r.runParallel(ctx, seeds, st)
	}
	r.Log.Log(MatchEndRecord{Type: "match_end", Complete: err == nil && st.Played() == r.Games, Standings: st.Table()})
	return st, err
}

// runParallel plays games on r.Parallel workers. The first failure stops
// the match; games in progress are abandoned.
func (r *Runner) runParallel(parent context.Context, seeds []uint64, st *Standings) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var (
		wg       sync.WaitGroup
		errMu    sync.Mutex
		firstErr error
	)
	jobs := make(chan int)
	for range r.Parallel {
		wg.Go(func() {
			for g := range jobs {
				res, err := r.playGame(ctx, g, seeds[g/4], st)
				if err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					cancel()
					continue
				}
				r.finishGame(g, res, st)
			}
		})
	}
feed:
	for g := range r.Games {
		select {
		case jobs <- g:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	if parent.Err() != nil {
		return parent.Err()
	}
	return firstErr
}

func (r *Runner) finishGame(g int, res GameResult, st *Standings) {
	r.mu.Lock()
	defer r.mu.Unlock()
	res = st.AddGame(g, res.Scores)
	r.Log.Log(GameEndRecord{Type: "game_end", GameResult: res})
	if r.OnGameEnd != nil {
		r.OnGameEnd(res, st)
	}
}

func (r *Runner) playGame(ctx context.Context, gameID int, dealSeed uint64, st *Standings) (GameResult, error) {
	rot := gameID % 4
	var names [4]string
	for seat := range 4 {
		names[seat] = r.Names[BotAt(seat, rot)]
	}
	r.Log.Log(GameStartRecord{Type: "game_start", Game: gameID, DealSet: gameID / 4, Rotation: rot, Seats: names})

	g := king.NewGame(king.Deals(dealSeed))
	for n := 0; !g.Done(); n++ {
		hand := g.HandNumber()
		if err := r.decide(ctx, g, gameID, n, rot, names, st); err != nil {
			return GameResult{}, err
		}
		if g.HandNumber() != hand {
			rec := g.History()[hand-1]
			r.Log.Log(HandEndRecord{Type: "hand_end", Game: gameID, Hand: hand, Declarer: rec.Declarer, Contract: rec.Contract, Scores: rec.Score})
		}
	}
	var res GameResult
	totals := g.Totals()
	for b := range 4 {
		res.Scores[b] = totals[SeatOf(b, rot)]
	}
	return res, nil
}

// decide asks the player whose turn it is for one decision and applies it,
// falling back to the lowest legal option on a timeout or an illegal answer.
func (r *Runner) decide(ctx context.Context, g *king.Game, gameID, n, rot int, names [4]string, st *Standings) error {
	seat := g.Turn()
	bot := BotAt(seat, rot)
	p := r.Players[bot]
	req := protocol.NewRequest(fmt.Sprintf("%d-%d", gameID, n), g, gameID, names)

	var resp *protocol.Response
	var err error
	for {
		dctx, cancel := context.WithTimeout(ctx, r.Timeout)
		resp, err = p.Decide(dctx, req)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !errors.Is(err, ErrDisconnected) {
			break
		}
		rc, ok := p.(Reconnector)
		if !ok {
			return fmt.Errorf("bot %s: %w", r.Names[bot], err)
		}
		r.pause(gameID, bot, true)
		if err := rc.WaitConnected(ctx); err != nil {
			return err
		}
		r.pause(gameID, bot, false)
	}

	rec := DecisionRecord{
		Type: "decision", Game: gameID, Hand: g.HandNumber(), Seat: seat, Bot: r.Names[bot],
		Kind: req.Kind, LegalCards: req.LegalCards, LegalContracts: req.LegalContracts,
	}
	v := Judge(req, resp, err)
	rec.Fault, rec.Detail, rec.Card, rec.Contract = v.Fault, v.Detail, v.Card, v.Contract
	if req.Kind == protocol.ChooseContract {
		if err := g.Choose(seat, *v.Contract); err != nil {
			return err // engine bug: legal contract rejected
		}
	} else if err := g.Play(seat, *v.Card); err != nil {
		return err
	}
	if rec.Fault != "" {
		r.mu.Lock()
		st.AddStrike(bot)
		r.mu.Unlock()
	}
	r.Log.Log(rec)
	return nil
}

// Verdict is the outcome of judging one answer.
type Verdict struct {
	// Fault is empty for a legal answer, else FaultTimeout, FaultIllegal or
	// FaultError.
	Fault  string
	Detail string
	// Card or Contract is the move to apply: the bot's answer if legal,
	// otherwise the lowest legal option.
	Card     *king.Card
	Contract *king.Contract
}

// Judge checks a player's answer to req. err is the error Decide returned.
func Judge(req *protocol.Request, resp *protocol.Response, err error) Verdict {
	var v Verdict
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		v.Fault = FaultTimeout
	case err != nil:
		v.Fault, v.Detail = FaultError, err.Error()
	case resp == nil || resp.ID != req.ID:
		v.Fault, v.Detail = FaultIllegal, "missing response or wrong id"
	}
	if req.Kind == protocol.ChooseContract {
		c := req.LegalContracts[0]
		if v.Fault == "" {
			if resp.Contract != nil && slices.Contains(req.LegalContracts, *resp.Contract) {
				c = *resp.Contract
			} else {
				v.Fault, v.Detail = FaultIllegal, fmt.Sprintf("contract %v is not legal", describe(resp.Contract))
			}
		}
		v.Contract = &c
		return v
	}
	c := king.Lowest(req.LegalCards)
	if v.Fault == "" {
		if resp.Card != nil && slices.Contains(req.LegalCards, *resp.Card) {
			c = *resp.Card
		} else {
			v.Fault, v.Detail = FaultIllegal, fmt.Sprintf("card %v is not legal", describe(resp.Card))
		}
	}
	v.Card = &c
	return v
}

func describe[T fmt.Stringer](p *T) string {
	if p == nil {
		return "(missing)"
	}
	return (*p).String()
}

func (r *Runner) pause(gameID, bot int, paused bool) {
	typ := "resume"
	if paused {
		typ = "pause"
	}
	r.Log.Log(PauseRecord{Type: typ, Game: gameID, Bot: r.Names[bot]})
	if r.OnPause != nil {
		r.OnPause(r.Names[bot], paused)
	}
}
