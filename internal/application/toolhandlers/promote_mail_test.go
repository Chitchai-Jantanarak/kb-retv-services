package toolhandlers

import (
	"context"
	"errors"
	"testing"

	"github.com/my/app/internal/application/skeleton"
)

type stubPromoter struct {
	code     string
	err      error
	called   bool
	coverage []int64
	convID   int64
	actorID  int64
}

func (s *stubPromoter) PromoteConversation(_ context.Context, coverage []int64, conversationID, actorEmployeeID int64) (string, error) {
	s.called = true
	s.coverage = coverage
	s.convID = conversationID
	s.actorID = actorEmployeeID
	return s.code, s.err
}

func TestPromoteMailDeniedWithoutReportCreate(t *testing.T) {
	repo := &stubPromoter{code: "REP-9"}
	h := NewPromoteMail(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "promote email 12",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.update"}, Coverage: []int64{3}},
	})
	if err == nil {
		t.Fatal("err = nil, want denial without report.create")
	}
	if repo.called {
		t.Fatal("PromoteConversation must not be called when denied")
	}
}

func TestPromoteMailNeedsID(t *testing.T) {
	repo := &stubPromoter{code: "REP-9"}
	h := NewPromoteMail(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "make a ticket from this email",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.create"}, Coverage: []int64{3}},
	})
	if !errors.Is(err, skeleton.ErrNeedsParam) {
		t.Fatalf("err = %v, want NeedsParams when no conversation id present", err)
	}
	if repo.called {
		t.Fatal("PromoteConversation must not be called without an id")
	}
}

func TestPromoteMailPrefersDeclaredParam(t *testing.T) {
	repo := &stubPromoter{code: "REP-42"}
	h := NewPromoteMail(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{
		Params:   map[string]string{"conversation_id": "7"},
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.create"}, Coverage: []int64{3}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 1 || repo.convID != 7 {
		t.Fatalf("convID = %d rows = %v, want 7 from params with empty text (confirm path)", repo.convID, rows)
	}
}

func TestPromoteMailAmbiguousNumbersAskInsteadOfGuessing(t *testing.T) {
	repo := &stubPromoter{code: "REP-42"}
	h := NewPromoteMail(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "in company 99 promote email 12",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.create"}, Coverage: []int64{3}},
	})
	if !errors.Is(err, skeleton.ErrNeedsParam) {
		t.Fatalf("err = %v, want NeedsParams on ambiguous numbers", err)
	}
	if repo.called {
		t.Fatal("PromoteConversation must not run on a guessed id")
	}
}

func TestPromoteMailSuccessMapsCode(t *testing.T) {
	repo := &stubPromoter{code: "REP-42"}
	h := NewPromoteMail(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{
		Text:     "create case from email 12",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.create"}, Coverage: []int64{3}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 1 || rows[0]["code"] != "REP-42" {
		t.Fatalf("rows = %v, want single row with code REP-42", rows)
	}
	if repo.convID != 12 {
		t.Fatalf("convID = %d, want 12", repo.convID)
	}
	if repo.actorID != 55 {
		t.Fatalf("actorID = %d, want 55 (from actor, not message)", repo.actorID)
	}
}

func TestPromoteMailUsesCoverageNotMessage(t *testing.T) {
	repo := &stubPromoter{code: "REP-42"}
	h := NewPromoteMail(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "in company 99 promote this email",
		Params:   map[string]string{"conversation_id": "12"},
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.create"}, Coverage: []int64{3}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if repo.convID != 12 {
		t.Fatalf("convID = %d, want 12 from declared param, never the company number", repo.convID)
	}
	if len(repo.coverage) != 1 || repo.coverage[0] != 3 {
		t.Fatalf("coverage = %v, want [3] from actor coverage", repo.coverage)
	}
}
