package rag

import (
	"context"
	"errors"
	"testing"
)

type stubRerankEmbedder struct {
	vectors map[string][]float32
	err     error
	short   bool
}

func (s *stubRerankEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.short {
		return [][]float32{{1, 0}}, nil
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v, ok := s.vectors[t]
		if !ok {
			v = []float32{0, 0}
		}
		out[i] = v
	}
	return out, nil
}

func TestEmbeddingRerankerOrdersByCosineAndDropsBelowFloor(t *testing.T) {
	r := EmbeddingReranker{
		Embedder: &stubRerankEmbedder{vectors: map[string][]float32{
			"query":    {1, 0},
			"off":      {0, 1},
			"on":       {1, 0},
			"halfway":  {1, 1},
		}},
		MinSimilarity: 0.5,
	}
	out, err := r.Rerank(context.Background(), Query{Text: "query"}, Meta{}, []Candidate{
		{ID: "a", Content: "off", Score: 20},
		{ID: "b", Content: "halfway", Score: 10},
		{ID: "c", Content: "on", Score: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].ID != "c" || out[1].ID != "b" {
		t.Fatalf("out = %+v, want [c b]", out)
	}
	if out[0].Score != 1 {
		t.Fatalf("out[0].Score = %v, want cosine 1", out[0].Score)
	}
}

func TestEmbeddingRerankerCapsAtLimit(t *testing.T) {
	vectors := map[string][]float32{"q": {1, 0}}
	cands := make([]Candidate, 0, 5)
	for i := range 5 {
		content := string(rune('a' + i))
		vectors[content] = []float32{1, 0}
		cands = append(cands, Candidate{ID: content, Content: content})
	}
	r := EmbeddingReranker{Embedder: &stubRerankEmbedder{vectors: vectors}, Limit: 3}
	out, err := r.Rerank(context.Background(), Query{Text: "q"}, Meta{}, cands)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 {
		t.Fatalf("len(out) = %d, want 3", len(out))
	}
}

func TestEmbeddingRerankerErrors(t *testing.T) {
	if _, err := (EmbeddingReranker{Embedder: &stubRerankEmbedder{err: errors.New("down")}}).Rerank(
		context.Background(), Query{Text: "q"}, Meta{}, []Candidate{{ID: "a", Content: "x"}},
	); err == nil {
		t.Fatal("want embed error")
	}
	if _, err := (EmbeddingReranker{Embedder: &stubRerankEmbedder{short: true}}).Rerank(
		context.Background(), Query{Text: "q"}, Meta{}, []Candidate{{ID: "a", Content: "x"}},
	); err == nil {
		t.Fatal("want vector-count mismatch error")
	}
}

func TestEmbeddingRerankerNilEmbedderPassesThrough(t *testing.T) {
	cands := []Candidate{{ID: "a", Score: 7}}
	out, err := (EmbeddingReranker{}).Rerank(context.Background(), Query{Text: "q"}, Meta{}, cands)
	if err != nil || len(out) != 1 || out[0].Score != 7 {
		t.Fatalf("out=%+v err=%v, want passthrough", out, err)
	}
}
