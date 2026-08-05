package tools

import (
	"context"
	"testing"

	"github.com/my/app/internal/shared/ctxkey"
)

func referenceFixture(t *testing.T) *Selector {
	t.Helper()
	catalog := []Tool{{
		ID:      "f2_case_status",
		Anchors: []string{"check status of case"},
		Params:  []Param{{Name: "code", Required: true, Pattern: `REP-\d+`}},
		Compose: Compose{Cite: "code"},
	}}
	sel, err := NewSelector(context.Background(), fixedEmbedder{vec: []float32{1, 0}}, catalog)
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}
	return sel
}

func TestSelectResolvesReferenceFromPriorTurns(t *testing.T) {
	sel := referenceFixture(t)
	ctx := ctxkey.WithCiteCandidates(context.Background(),
		[]string{"พบ 2 เคส\nREP-4106|robot stuck|waiting\nREP-4105|elevator|success"})

	got, err := sel.Select(ctx, "check status of case ล่าสุด", []string{"*"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if !got.Matched || got.Params["code"] != "REP-4106" {
		t.Fatalf("got %+v, want f2_case_status with code=REP-4106 resolved from the prior turn", got)
	}
}

func TestSelectLeavesReferenceUnresolvedWhenAmbiguous(t *testing.T) {
	sel := referenceFixture(t)
	ctx := ctxkey.WithCiteCandidates(context.Background(),
		[]string{"REP-4106|a|waiting\nREP-4105|b|waiting"})

	got, err := sel.Select(ctx, "check status of case อันนี้", []string{"*"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got.Matched {
		t.Fatalf("got %+v, want no match so the clarify path asks which case", got)
	}
}

func TestSelectWithoutPriorTurnsIsUnchanged(t *testing.T) {
	sel := referenceFixture(t)

	got, err := sel.Select(context.Background(), "check status of case ล่าสุด", []string{"*"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got.Matched {
		t.Fatalf("got %+v, want no match when there is nothing to resolve against", got)
	}
}
