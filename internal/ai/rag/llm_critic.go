package rag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/shared/ctxkey"

	"github.com/my/app/internal/shared/usagemeter"
)

type CritiqueResult struct {
	Supported      bool
	Send           bool
	Hallucinations []string
	Refusal        string
}

type Critic interface {
	Critique(ctx context.Context, query Query, draft string, candidates []Candidate) (CritiqueResult, error)
}

type LLMCritic struct {
	registry      *prompts.Registry
	resolve       ProviderForCompany
	maxCandidates int
}

func NewLLMCritic(registry *prompts.Registry, resolve ProviderForCompany) (*LLMCritic, error) {
	if registry == nil {
		return nil, errors.New("rag: llm critic: registry is required")
	}
	if resolve == nil {
		return nil, errors.New("rag: llm critic: resolver is required")
	}
	return &LLMCritic{registry: registry, resolve: resolve, maxCandidates: 5}, nil
}

func (c *LLMCritic) Critique(ctx context.Context, query Query, draft string, candidates []Candidate) (CritiqueResult, error) {
	if strings.TrimSpace(draft) == "" {
		return CritiqueResult{Supported: false, Send: false, Refusal: "empty draft"}, nil
	}
	companyID, ok := ctxkey.CompanyID(ctx)
	if !ok || companyID <= 0 {
		return CritiqueResult{Supported: true, Send: true}, nil
	}

	tmpl, err := c.registry.Get(prompts.NameSelfRAG)
	if err != nil {
		return CritiqueResult{}, fmt.Errorf("rag: llm critic: get template: %w", err)
	}
	evidence := formatEvidence(candidates, c.maxCandidates)
	rendered, err := tmpl.Render(map[string]string{
		"query":    query.Text,
		"draft":    draft,
		"evidence": evidence,
	})
	if err != nil {
		return CritiqueResult{}, fmt.Errorf("rag: llm critic: render: %w", err)
	}

	provider, err := c.resolve(ctx, companyID)
	if err != nil {
		return CritiqueResult{}, fmt.Errorf("rag: llm critic: resolve: %w", err)
	}
	completion, err := provider.GenerateJSON(ctx, rendered)
	if err != nil {
		return CritiqueResult{}, fmt.Errorf("rag: llm critic: generate: %w", err)
	}
	usagemeter.Add(ctx, "critique", completion.Usage)

	var parsed struct {
		Supported      bool     `json:"supported"`
		Hallucinations []string `json:"hallucinations"`
		RefusalReason  string   `json:"refusal_reason"`
		Send           bool     `json:"send"`
	}
	if err := json.Unmarshal([]byte(extractJSONObject(completion.Text)), &parsed); err != nil {
		return CritiqueResult{}, fmt.Errorf("rag: llm critic: parse %q: %w", completion.Text, err)
	}

	return CritiqueResult{
		Supported:      parsed.Supported,
		Send:           parsed.Send,
		Hallucinations: parsed.Hallucinations,
		Refusal:        parsed.RefusalReason,
	}, nil
}
