package rag

import (
	"context"
	"fmt"
	"sort"

	"github.com/my/app/internal/shared/vec"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type EmbeddingReranker struct {
	Embedder      Embedder
	MinSimilarity float64
	Limit         int
}

func (r EmbeddingReranker) Rerank(ctx context.Context, query Query, _ Meta, candidates []Candidate) ([]Candidate, error) {
	if r.Embedder == nil {
		return candidates, nil
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	texts := make([]string, 0, len(candidates)+1)
	texts = append(texts, query.Text)
	for _, c := range candidates {
		texts = append(texts, c.Content)
	}
	vectors, err := r.Embedder.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(texts) {
		return nil, fmt.Errorf("rag: embedding rerank: embed returned %d vectors, want %d", len(vectors), len(texts))
	}
	queryVec := vec.Normalize(vectors[0])
	out := make([]Candidate, 0, len(candidates))
	for i, c := range candidates {
		score := vec.Dot(queryVec, vec.Normalize(vectors[i+1]))
		if score < r.MinSimilarity {
			continue
		}
		c.Score = score
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	if r.Limit > 0 && len(out) > r.Limit {
		out = out[:r.Limit]
	}
	return out, nil
}

var _ Reranker = EmbeddingReranker{}
