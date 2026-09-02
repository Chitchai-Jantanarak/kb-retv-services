package rag

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/shared/ctxkey"

	"github.com/my/app/internal/shared/usagemeter"
)

type Generator interface {
	Generate(ctx context.Context, query Query, candidates []Candidate) (string, error)
}

type LLMGenerator struct {
	registry      *prompts.Registry
	resolve       ProviderForCompany
	defaultTone   string
	maxSentences  string
	maxCandidates int
}

type LLMGeneratorConfig struct {
	Registry      *prompts.Registry
	Resolve       ProviderForCompany
	Tone          string
	MaxSentences  int
	MaxCandidates int
}

func NewLLMGenerator(cfg LLMGeneratorConfig) (*LLMGenerator, error) {
	if cfg.Registry == nil {
		return nil, errors.New("rag: llm generator: registry is required")
	}
	if cfg.Resolve == nil {
		return nil, errors.New("rag: llm generator: resolver is required")
	}
	tone := strings.TrimSpace(cfg.Tone)
	if tone == "" {
		tone = "professional and concise"
	}
	maxSentences := cfg.MaxSentences
	if maxSentences <= 0 {
		maxSentences = 4
	}
	maxCandidates := cfg.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 5
	}
	return &LLMGenerator{
		registry:      cfg.Registry,
		resolve:       cfg.Resolve,
		defaultTone:   tone,
		maxSentences:  strconv.Itoa(maxSentences),
		maxCandidates: maxCandidates,
	}, nil
}

func (g *LLMGenerator) Generate(ctx context.Context, query Query, candidates []Candidate) (string, error) {
	if len(candidates) == 0 {
		return "", nil
	}
	companyID, ok := ctxkey.CompanyID(ctx)
	if !ok || companyID <= 0 {
		return "", nil
	}

	tmpl, err := g.registry.Get(prompts.NameGenerate)
	if err != nil {
		return "", fmt.Errorf("rag: llm generator: get template: %w", err)
	}

	evidence := formatEvidence(candidates, g.maxCandidates)
	rendered, err := tmpl.Render(map[string]string{
		"query":         query.Text,
		"evidence":      evidence,
		"tone":          g.defaultTone,
		"max_sentences": g.maxSentences,
	})
	if err != nil {
		return "", fmt.Errorf("rag: llm generator: render: %w", err)
	}

	provider, err := g.resolve(ctx, companyID)
	if err != nil {
		return "", fmt.Errorf("rag: llm generator: resolve: %w", err)
	}
	completion, err := provider.Generate(ctx, rendered)
	if err != nil {
		return "", fmt.Errorf("rag: llm generator: generate: %w", err)
	}
	usagemeter.Add(ctx, "generate", completion.Usage)
	return strings.TrimSpace(completion.Text), nil
}

func formatEvidence(candidates []Candidate, maxCandidates int) string {
	limit := len(candidates)
	if maxCandidates > 0 && limit > maxCandidates {
		limit = maxCandidates
	}
	var b strings.Builder
	for i := 0; i < limit; i++ {
		c := candidates[i]
		b.WriteString("[src:")
		b.WriteString(c.ID)
		b.WriteString("] ")
		if c.Title != "" {
			b.WriteString(c.Title)
			b.WriteString(" — ")
		}
		b.WriteString(truncate(c.Content, 600))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
