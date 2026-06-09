package rag

import (
	"context"
	"sort"
	"strings"
)

type LexicalReranker struct{}

func (LexicalReranker) Rerank(_ context.Context, query Query, meta Meta, candidates []Candidate) ([]Candidate, error) {
	out := make([]Candidate, len(candidates))
	copy(out, candidates)

	for i := range out {
		out[i].Score += lexicalScore(meta.Terms, out[i])
	}

	sort.SliceStable(out, func(i int, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	return out, nil
}

func lexicalScore(terms []string, candidate Candidate) float64 {
	text := strings.ToLower(candidate.Title + " " + candidate.Content)
	var score float64
	for _, term := range terms {
		if strings.Contains(text, term) {
			score++
		}
	}
	return score
}
