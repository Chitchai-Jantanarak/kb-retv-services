package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/domain/ports"
)

type AgentConfig struct {
	ID                  int64
	Vendor              string
	Model               string
	APIKey              string
	BaseURL             string
	ConfidenceThreshold float64
}

type AgentLookup interface {
	AgentFor(ctx context.Context, companyID int64) (AgentConfig, bool, error)
}

type NoopAgentLookup struct{}

func (NoopAgentLookup) AgentFor(_ context.Context, _ int64) (AgentConfig, bool, error) {
	return AgentConfig{}, false, nil
}

type CompanyResolver struct {
	lookup           AgentLookup
	defaults         Settings
	defaultThreshold float64
}

const DefaultConfidenceThreshold = 0.85

func NewCompanyResolver(lookup AgentLookup, defaults Settings) (*CompanyResolver, error) {
	if strings.TrimSpace(defaults.Vendor) == "" {
		return nil, errors.New("llm: company resolver: defaults.Vendor is required")
	}
	if lookup == nil {
		lookup = NoopAgentLookup{}
	}
	return &CompanyResolver{
		lookup:           lookup,
		defaults:         defaults,
		defaultThreshold: DefaultConfidenceThreshold,
	}, nil
}

func (r *CompanyResolver) WithDefaultThreshold(t float64) *CompanyResolver {
	if t > 0 && t <= 1 {
		r.defaultThreshold = t
	}
	return r
}

func (r *CompanyResolver) AgentFor(ctx context.Context, companyID int64) (AgentConfig, error) {
	if companyID <= 0 {
		return AgentConfig{}, errors.New("llm: company resolver: company_id must be positive")
	}

	cfg := AgentConfig{
		Vendor:              r.defaults.Vendor,
		Model:               r.defaults.Model,
		ConfidenceThreshold: r.defaultThreshold,
	}

	agent, ok, err := r.lookup.AgentFor(ctx, companyID)
	if err != nil {
		return AgentConfig{}, fmt.Errorf("llm: company resolver: lookup company %d: %w", companyID, err)
	}
	if ok {
		cfg.ID = agent.ID
		if v := strings.TrimSpace(agent.Vendor); v != "" && !strings.EqualFold(v, "default") {
			cfg.Vendor = v
		}
		if m := strings.TrimSpace(agent.Model); m != "" {
			cfg.Model = m
		}
		if agent.ConfidenceThreshold > 0 {
			cfg.ConfidenceThreshold = agent.ConfidenceThreshold
		}
		cfg.APIKey = agent.APIKey
		cfg.BaseURL = agent.BaseURL
	}
	return cfg, nil
}

func (r *CompanyResolver) ResolveFor(ctx context.Context, companyID int64) (ports.LLMProvider, error) {
	cfg, err := r.AgentFor(ctx, companyID)
	if err != nil {
		return nil, err
	}
	settings := r.defaults
	settings.Vendor = cfg.Vendor
	settings.Model = cfg.Model
	if cfg.APIKey != "" {
		applyVendorKey(&settings, cfg.Vendor, cfg.APIKey)
	}
	if cfg.BaseURL != "" {
		settings.OpenAIBaseURL = cfg.BaseURL
	}
	return Resolve(settings)
}

func applyVendorKey(s *Settings, vendor, key string) {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case VendorOpenAI:
		s.OpenAIKey = key
	case VendorOpenRouter:
		s.OpenRouterKey = key
	case VendorAnthropic, VendorClaude:
		s.AnthropicKey = key
	case VendorGemini:
		s.GeminiKey = key
	}
}
