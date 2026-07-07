package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/shared/ctxkey"
)

type scorerFTS struct {
	chunks   []rag.FTSChunk
	err      error
	coverage []int64
	limit    int
}

func (s *scorerFTS) SearchChunks(_ context.Context, coverage []int64, _ string, limit int) ([]rag.FTSChunk, error) {
	s.coverage = coverage
	s.limit = limit
	return s.chunks, nil
}

func TestFTSScorerReturnsTopRelevanceForCompany(t *testing.T) {
	fts := &scorerFTS{chunks: []rag.FTSChunk{{ChunkID: 1, Relevance: 4.2}}}
	scorer := newFTSScorer(fts)

	score, err := scorer.TopScore(ctxkey.WithCompanyID(context.Background(), 3), "robot")
	if err != nil {
		t.Fatalf("TopScore() error = %v", err)
	}
	if score != 4.2 {
		t.Fatalf("score = %v, want 4.2", score)
	}
	if fts.coverage[0] != 3 {
		t.Fatalf("coverage = %#v, want [3]", fts.coverage)
	}
	if fts.limit != 1 {
		t.Fatalf("limit = %d, want 1", fts.limit)
	}
}

func TestFTSScorerReturnsZeroWithoutCompanyScope(t *testing.T) {
	scorer := newFTSScorer(&scorerFTS{chunks: []rag.FTSChunk{{Relevance: 9}}})

	score, err := scorer.TopScore(context.Background(), "robot")
	if err != nil {
		t.Fatalf("TopScore() error = %v", err)
	}
	if score != 0 {
		t.Fatalf("score = %v, want 0 when no company scope", score)
	}
}

func TestFTSScorerReturnsZeroWhenNoChunks(t *testing.T) {
	scorer := newFTSScorer(&scorerFTS{})

	score, err := scorer.TopScore(ctxkey.WithCompanyID(context.Background(), 3), "robot")
	if err != nil {
		t.Fatalf("TopScore() error = %v", err)
	}
	if score != 0 {
		t.Fatalf("score = %v, want 0", score)
	}
}

func TestFTSScorerPropagatesError(t *testing.T) {
	scorer := newFTSScorer(errFTS{err: errors.New("fts down")})

	if _, err := scorer.TopScore(ctxkey.WithCompanyID(context.Background(), 3), "robot"); err == nil {
		t.Fatal("TopScore() error = nil, want fts error")
	}
}

type errFTS struct{ err error }

func (e errFTS) SearchChunks(context.Context, []int64, string, int) ([]rag.FTSChunk, error) {
	return nil, e.err
}
