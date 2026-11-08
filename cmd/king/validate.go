package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/cenkalti/king-server/match"
	"github.com/cenkalti/king-server/protocol"
	"github.com/cenkalti/king-server/server"
)

// runValidate waits for one bot and sends it every validation scenario.
func runValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:8080", "listen address")
	timeout := fs.Duration("timeout", 2*time.Second, "time the bot has for each answer")
	fs.Parse(args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	hub := server.NewHub(1)
	hub.Logf = logf
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/ws", hub)
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	defer srv.Close()
	logf("listening on ws://%s/ws; waiting for one bot...", ln.Addr())
	select {
	case <-hub.Ready():
	case <-ctx.Done():
		return errors.New("interrupted while waiting for a bot")
	}
	p := hub.Start()[0]
	results, err := validate(ctx, p, p.Name(), *timeout)
	p.SendMatchEnd(nil)
	fmt.Print(formatValidation(results))
	if err != nil {
		return err
	}
	for _, r := range results {
		if r.Result != "legal" {
			return errors.New("bot failed validation")
		}
	}
	return nil
}

type validationResult struct {
	Scenario string
	Result   string // legal, illegal, timeout, error
	Detail   string
}

// validate asks p about every scenario. A disconnect waits for the bot to
// come back and asks again.
func validate(ctx context.Context, p match.Player, name string, timeout time.Duration) ([]validationResult, error) {
	var out []validationResult
	for i, sc := range match.Scenarios() {
		g, err := sc.Game()
		if err != nil {
			return out, fmt.Errorf("scenario %q: %w", sc.Name, err)
		}
		var names [4]string
		for s := range 4 {
			names[s] = fmt.Sprintf("house-%d", s)
		}
		names[g.Turn()] = name
		req := protocol.NewRequest(fmt.Sprintf("validate-%d", i), g, i, names)

		var resp *protocol.Response
		for {
			dctx, cancel := context.WithTimeout(ctx, timeout)
			resp, err = p.Decide(dctx, req)
			cancel()
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			rc, ok := p.(match.Reconnector)
			if !errors.Is(err, match.ErrDisconnected) || !ok {
				break
			}
			logf("bot disconnected; waiting for it to reconnect")
			if err := rc.WaitConnected(ctx); err != nil {
				return out, err
			}
		}
		v := match.Judge(req, resp, err)
		r := validationResult{Scenario: sc.Name, Result: "legal", Detail: answerString(req, resp)}
		if v.Fault != "" {
			r.Result = v.Fault
			if v.Detail != "" {
				r.Detail = v.Detail
			}
		}
		out = append(out, r)
	}
	return out, nil
}

func answerString(req *protocol.Request, resp *protocol.Response) string {
	if resp == nil {
		return ""
	}
	if req.Kind == protocol.ChooseContract && resp.Contract != nil {
		return resp.Contract.String()
	}
	if resp.Card != nil {
		return resp.Card.String()
	}
	return ""
}

func formatValidation(results []validationResult) string {
	var b strings.Builder
	pass := 0
	for _, r := range results {
		mark := "ok  "
		if r.Result == "legal" {
			pass++
		} else {
			mark = "FAIL"
		}
		fmt.Fprintf(&b, "%s  %-55s  %-8s %s\n", mark, r.Scenario, r.Result, r.Detail)
	}
	fmt.Fprintf(&b, "\n%d/%d scenarios answered legally\n", pass, len(results))
	return b.String()
}
