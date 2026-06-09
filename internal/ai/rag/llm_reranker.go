package rag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/shared/ctxkey"
)

type LLMReranker struct {
	registry *prompts.Registry
	resolve  ProviderForCompany
	fallback Reranker
}

func NewLLMReranker(registry *prompts.Registry, resolve ProviderForCompany, fallback Reranker) (*LLMReranker, error) {
	if registry == nil {
		return nil, errors.New("rag: llm reranker: registry is required")
	}
	if resolve == nil {
		return nil, errors.New("rag: llm reranker: resolver is required")
	}
	if fallback == nil {
		fallback = LexicalReranker{}
	}
	return &LLMReranker{registry: registry, resolve: resolve, fallback: fallback}, nil
}

func (r *LLMReranker) Rerank(ctx context.Context, query Query, meta Meta, candidates []Candidate) ([]Candidate, error) {
	if len(candidates) <= 1 {
		return r.fallback.Rerank(ctx, query, meta, candidates)
	}
	companyID, ok := ctxkey.CompanyID(ctx)
	if !ok || companyID <= 0 {
		return r.fallback.Rerank(ctx, query, meta, candidates)
	}

	tmpl, err := r.registry.Get(prompts.NameRerank)
	if err != nil {
		return nil, fmt.Errorf("rag: llm reranker: get template: %w", err)
	}
	rendered, err := tmpl.Render(map[string]string{
		"query":      query.Text,
		"candidates": formatCandidates(candidates),
	})
	if err != nil {
		return nil, fmt.Errorf("rag: llm reranker: render: %w", err)
	}

	provider, err := r.resolve(ctx, companyID)
	if err != nil {
		return r.fallback.Rerank(ctx, query, meta, candidates)
	}

	completion, err := provider.GenerateJSON(ctx, rendered)
	if err != nil {
		return r.fallback.Rerank(ctx, query, meta, candidates)
	}

	var parsed struct {
		Scores []struct {
			ID     string  `json:"id"`
			Score  float64 `json:"score"`
			Reason string  `json:"reason"`
		} `json:"scores"`
	}
	if err := json.Unmarshal([]byte(completion.Text), &parsed); err != nil {
		return r.fallback.Rerank(ctx, query, meta, candidates)
	}

	scores := make(map[string]float64, len(parsed.Scores))
	for _, s := range parsed.Scores {
		id := strings.TrimSpace(s.ID)
		if id == "" {
			continue
		}
		scores[id] = s.Score
	}
	if len(scores) == 0 {
		return r.fallback.Rerank(ctx, query, meta, candidates)
	}

	out := make([]Candidate, len(candidates))
	copy(out, candidates)
	for i := range out {
		if s, ok := scores[out[i].ID]; ok {
			out[i].Score = s
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].ID < out[j].ID
		}
		return out[i].Score > out[j].Score
	})
	return out, nil
}

func formatCandidates(candidates []Candidate) string {
	var b strings.Builder
	for _, c := range candidates {
		b.WriteString(c.ID)
		b.WriteString(": ")
		b.WriteString(truncate(c.Title, 80))
		if c.Content != "" {
			b.WriteString(" — ")
			b.WriteString(truncate(c.Content, 240))
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func truncate(s string, maxLen int) string {
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen]) + "…"
}
