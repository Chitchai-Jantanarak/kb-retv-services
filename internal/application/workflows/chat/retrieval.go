package chat

import (
	"context"

	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/shared/ctxkey"
)

type ftsScorer struct {
	fts rag.FTSSource
}

func newFTSScorer(fts rag.FTSSource) *ftsScorer {
	return &ftsScorer{fts: fts}
}

func NewFTSScorer(fts rag.FTSSource) *ftsScorer {
	return newFTSScorer(fts)
}

func (s *ftsScorer) TopScore(ctx context.Context, query string) (float64, error) {
	if s.fts == nil {
		return 0, nil
	}
	companyID, ok := ctxkey.CompanyID(ctx)
	if !ok || companyID <= 0 {
		return 0, nil
	}
	chunks, err := s.fts.SearchChunks(ctx, []int64{companyID}, query, 1)
	if err != nil {
		return 0, err
	}
	if len(chunks) == 0 {
		return 0, nil
	}
	return chunks[0].Relevance, nil
}
