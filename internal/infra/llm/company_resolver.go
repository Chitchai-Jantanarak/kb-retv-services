package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

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

	resilientOpts ResilientOptions
	ttl           time.Duration
	mu            sync.Mutex
	cache         map[int64]cacheEntry
}

type cacheEntry struct {
	cfg      AgentConfig
	provider ports.LLMProvider
	expires  time.Time
}

const DefaultConfidenceThreshold = 0.85

var ErrAIDisabled = ports.ErrAIDisabled

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

func (r *CompanyResolver) WithResilient(opts ResilientOptions) *CompanyResolver {
	r.resilientOpts = opts
	return r
}

func (r *CompanyResolver) WithCacheTTL(ttl time.Duration) *CompanyResolver {
	if ttl > 0 {
		r.ttl = ttl
		r.cache = make(map[int64]cacheEntry)
	}
	return r
}

func (r *CompanyResolver) cacheEnabled() bool { return r.ttl > 0 && r.cache != nil }

func (r *CompanyResolver) getEntry(companyID int64) (cacheEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.cache[companyID]
	if !ok || time.Now().After(e.expires) {
		return cacheEntry{}, false
	}
	return e, true
}

func (r *CompanyResolver) store(companyID int64, e cacheEntry) {
	e.expires = time.Now().Add(r.ttl)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[companyID] = e
}

func (r *CompanyResolver) AgentFor(ctx context.Context, companyID int64) (AgentConfig, error) {
	if companyID <= 0 {
		return AgentConfig{}, errors.New("llm: company resolver: company_id must be positive")
	}
	if r.cacheEnabled() {
		if e, ok := r.getEntry(companyID); ok {
			return e.cfg, nil
		}
	}
	cfg, err := r.lookupAgent(ctx, companyID)
	if err != nil {
		return AgentConfig{}, err
	}
	if r.cacheEnabled() {
		r.store(companyID, cacheEntry{cfg: cfg})
	}
	return cfg, nil
}

// No active ai_agents row means AI is off for the silo: never borrow the platform provider.
func (r *CompanyResolver) lookupAgent(ctx context.Context, companyID int64) (AgentConfig, error) {
	agent, ok, err := r.lookup.AgentFor(ctx, companyID)
	if err != nil {
		return AgentConfig{}, fmt.Errorf("llm: company resolver: lookup company %d: %w", companyID, err)
	}
	if !ok {
		return AgentConfig{}, ErrAIDisabled
	}

	cfg := AgentConfig{
		ID:                  agent.ID,
		Vendor:              r.defaults.Vendor,
		Model:               r.defaults.Model,
		ConfidenceThreshold: r.defaultThreshold,
	}
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

	return cfg, nil
}

func (r *CompanyResolver) ResolveFor(ctx context.Context, companyID int64) (ports.LLMProvider, error) {
	if companyID <= 0 {
		return nil, errors.New("llm: company resolver: company_id must be positive")
	}

	var (
		cfg    AgentConfig
		gotCfg bool
	)
	if r.cacheEnabled() {
		if e, ok := r.getEntry(companyID); ok {
			if e.provider != nil {
				return e.provider, nil
			}
			cfg, gotCfg = e.cfg, true
		}
	}
	if !gotCfg {
		c, err := r.lookupAgent(ctx, companyID)
		if err != nil {
			return nil, err
		}
		cfg = c
	}

	provider, err := r.build(cfg)
	if err != nil {
		return nil, err
	}
	if r.cacheEnabled() {
		r.store(companyID, cacheEntry{cfg: cfg, provider: provider})
	}
	return provider, nil
}

func (r *CompanyResolver) build(cfg AgentConfig) (ports.LLMProvider, error) {
	settings := r.defaults
	settings.Vendor = cfg.Vendor
	settings.Model = cfg.Model
	if cfg.APIKey != "" {
		applyVendorKey(&settings, cfg.Vendor, cfg.APIKey)
	}
	if cfg.BaseURL != "" {
		settings.OpenAIBaseURL = cfg.BaseURL
	}
	provider, err := Resolve(settings)
	if err != nil {
		return nil, err
	}
	return NewResilient(provider, r.resilientOpts), nil
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
	case VendorLocal:
		s.LocalKey = key
	}
}
