package rag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

type ProviderForCompany func(ctx context.Context, companyID int64) (ports.LLMProvider, error)

type LLMCRAG struct {
	registry *prompts.Registry
	resolve  ProviderForCompany
}

func NewLLMCRAG(registry *prompts.Registry, resolve ProviderForCompany) (*LLMCRAG, error) {
	if registry == nil {
		return nil, errors.New("rag: llm crag: registry is required")
	}
	if resolve == nil {
		return nil, errors.New("rag: llm crag: resolver is required")
	}
	return &LLMCRAG{registry: registry, resolve: resolve}, nil
}

func (c *LLMCRAG) Grade(ctx context.Context, query, content string) (CRAGResult, error) {
	if strings.TrimSpace(content) == "" {
		return CRAGResult{Verdict: VerdictIrrelevant}, nil
	}
	companyID, ok := ctxkey.CompanyID(ctx)
	if !ok || companyID <= 0 {
		return CRAGResult{}, errors.New("rag: llm crag: company_id missing from context")
	}

	tmpl, err := c.registry.Get(prompts.NameCRAG)
	if err != nil {
		return CRAGResult{}, fmt.Errorf("rag: llm crag: get template: %w", err)
	}
	rendered, err := tmpl.Render(map[string]string{
		"query":    query,
		"evidence": content,
	})
	if err != nil {
		return CRAGResult{}, fmt.Errorf("rag: llm crag: render: %w", err)
	}

	provider, err := c.resolve(ctx, companyID)
	if err != nil {
		return CRAGResult{}, fmt.Errorf("rag: llm crag: resolve provider: %w", err)
	}

	completion, err := provider.GenerateJSON(ctx, rendered)
	if err != nil {
		return CRAGResult{}, fmt.Errorf("rag: llm crag: generate: %w", err)
	}

	var parsed struct {
		Verdict    string  `json:"verdict"`
		Confidence float64 `json:"confidence"`
		Missing    string  `json:"missing"`
	}
	if err := json.Unmarshal([]byte(completion.Text), &parsed); err != nil {
		return CRAGResult{}, fmt.Errorf("rag: llm crag: parse %q: %w", completion.Text, err)
	}

	result := CRAGResult{
		Confidence: clamp01(parsed.Confidence),
		Missing:    strings.TrimSpace(parsed.Missing),
		Graded:     true,
	}
	switch strings.ToLower(strings.TrimSpace(parsed.Verdict)) {
	case "relevant":
		result.Verdict = VerdictRelevant
		return result, nil
	case "partial":
		result.Verdict = VerdictPartial
		return result, nil
	case "irrelevant":
		result.Verdict = VerdictIrrelevant
		return result, nil
	default:
		return CRAGResult{}, fmt.Errorf("rag: llm crag: unknown verdict %q", parsed.Verdict)
	}
}
