package toolhandlers

import (
	"context"
	"testing"

	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/application/tools"
	channelsmysql "github.com/my/app/internal/repositories/channels/mysql"
)

type stubInboundRepo struct {
	drafts   []channelsmysql.InboundDraftRow
	coverage []int64
	limit    int
}

func (s *stubInboundRepo) ListInboundDrafts(_ context.Context, coverage []int64, limit int) ([]channelsmysql.InboundDraftRow, error) {
	s.coverage = coverage
	s.limit = limit
	return s.drafts, nil
}

func TestInboundReadMapsDraftsToRows(t *testing.T) {
	t.Parallel()
	repo := &stubInboundRepo{drafts: []channelsmysql.InboundDraftRow{
		{ID: 9, Subject: "printer broken", From: "acme@example.com", Received: "2026-07-09 10:00:00"},
	}}
	h := NewInboundRead(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{
		Coverage: []int64{3},
		Tool:     tools.Tool{Limit: 10},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row["subject"] != "printer broken" || row["from"] != "acme@example.com" ||
		row["received"] != "2026-07-09 10:00:00" || row["id"] != "9" {
		t.Fatalf("row = %v, unexpected mapping", row)
	}
}

func TestInboundReadDefaultsLimit(t *testing.T) {
	t.Parallel()
	repo := &stubInboundRepo{}
	h := NewInboundRead(repo)
	if _, err := h.Run(context.Background(), skeleton.Query{Tool: tools.Tool{Limit: 0}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if repo.limit != 10 {
		t.Fatalf("limit = %d, want default 10", repo.limit)
	}
}

func TestInboundReadUsesCoverageNotMessage(t *testing.T) {
	t.Parallel()
	repo := &stubInboundRepo{}
	h := NewInboundRead(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "show emails for company 99",
		Coverage: []int64{3},
		Tool:     tools.Tool{Limit: 10},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(repo.coverage) != 1 || repo.coverage[0] != 3 {
		t.Fatalf("coverage = %v, want [3]", repo.coverage)
	}
}
