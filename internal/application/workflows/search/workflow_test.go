package search

import (
	"context"
	"strings"
	"testing"

	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
)

func TestWorkflowRunRequiresCompanyScope(t *testing.T) {
	w := NewWorkflow(&stubFTS{})
	_, err := w.Run(context.Background(), dto.SearchRequest{Query: "printer"})
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeUnauthorized {
		t.Fatalf("Run() error = %v, want unauthorized", err)
	}
}

func TestWorkflowRunPropagatesValidationError(t *testing.T) {
	w := NewWorkflow(&stubFTS{})
	ctx := ctxkey.WithCompanyID(context.Background(), 3)
	_, err := w.Run(ctx, dto.SearchRequest{Query: "   "})
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeInvalidInput {
		t.Fatalf("Run() error = %v, want invalid_input", err)
	}
}

func TestWorkflowRunReturnsEmptyResultsWhenFTSNil(t *testing.T) {
	w := NewWorkflow(nil)
	ctx := ctxkey.WithCompanyID(context.Background(), 3)
	resp, err := w.Run(ctx, dto.SearchRequest{Query: "printer"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(resp.Results) != 0 {
		t.Fatalf("Results = %v, want empty", resp.Results)
	}
	if resp.Query != "printer" {
		t.Fatalf("Query = %q, want printer", resp.Query)
	}
}

func TestWorkflowRunMapsChunksToResults(t *testing.T) {
	stub := &stubFTS{chunks: []rag.FTSChunk{{
		ChunkID:   9,
		ArticleID: 42,
		Title:     "Printer offline",
		Content:   strings.Repeat("a", 400),
		Relevance: 0.87,
	}}}
	w := NewWorkflow(stub)
	ctx := ctxkey.WithCompanyID(context.Background(), 3)

	resp, err := w.Run(ctx, dto.SearchRequest{Query: "printer offline", Limit: 5})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(stub.coverage) != 1 || stub.coverage[0] != 3 {
		t.Fatalf("coverage = %v, want [3]", stub.coverage)
	}
	if stub.query != "printer offline" || stub.limit != 5 {
		t.Fatalf("query/limit = %q/%d, want printer offline/5", stub.query, stub.limit)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(resp.Results))
	}
	got := resp.Results[0]
	if got.Type != "kb" || got.ID != "chunk:9" || got.ArticleID != "42" || got.Title != "Printer offline" {
		t.Fatalf("result = %+v, want type kb id chunk:9 article 42 title Printer offline", got)
	}
	wantRunes := 320
	gotRunes := []rune(got.Snippet)
	if !strings.HasSuffix(got.Snippet, "...") || len(gotRunes) != wantRunes+3 {
		t.Fatalf("snippet = %q (len %d), want %d runes + ellipsis", got.Snippet, len(gotRunes), wantRunes)
	}
	if got.Score != 0.87 {
		t.Fatalf("score = %v, want 0.87", got.Score)
	}
}

type stubFTS struct {
	chunks   []rag.FTSChunk
	coverage []int64
	query    string
	limit    int
}

func (s *stubFTS) SearchChunks(_ context.Context, coverage []int64, query string, limit int) ([]rag.FTSChunk, error) {
	s.coverage = coverage
	s.query = query
	s.limit = limit
	return s.chunks, nil
}
