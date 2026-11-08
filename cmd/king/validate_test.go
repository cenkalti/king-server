package main

import (
	"context"
	"testing"
	"time"

	"github.com/cenkalti/king-server/match"
	"github.com/cenkalti/king-server/protocol"
)

// careless plays the first card in its hand and the first contract, ignoring
// the legal lists for cards.
type careless struct{}

func (careless) Decide(_ context.Context, req *protocol.Request) (*protocol.Response, error) {
	resp := &protocol.Response{Type: protocol.TypeResponse, ID: req.ID}
	if req.Kind == protocol.ChooseContract {
		resp.Contract = &req.LegalContracts[0]
	} else {
		resp.Card = &req.State.Cards[0]
	}
	return resp, nil
}

func TestValidate(t *testing.T) {
	ctx := context.Background()
	for _, p := range []match.Player{match.LowBot{}, match.NewRandomBot(1)} {
		res, err := validate(ctx, p, "bot", time.Second)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range res {
			if r.Result != "legal" {
				t.Fatalf("%T failed %q: %s %s", p, r.Scenario, r.Result, r.Detail)
			}
		}
	}

	res, err := validate(ctx, careless{}, "bot", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	failed := 0
	for _, r := range res {
		if r.Result == match.FaultIllegal {
			failed++
		}
	}
	if failed == 0 {
		t.Fatal("careless bot passed validation")
	}
	t.Logf("careless bot:\n%s", formatValidation(res))
}
