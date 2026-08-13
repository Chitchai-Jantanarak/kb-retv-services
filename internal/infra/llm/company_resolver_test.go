package llm

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/my/app/internal/domain/ports"
)

type stubLookup struct {
	agent AgentConfig
	ok    bool
	err   error
}

func (s stubLookup) AgentFor(_ context.Context, _ int64) (AgentConfig, bool, error) {
	return s.agent, s.ok, s.err
}

type countingLookup struct {
	agent AgentConfig
	ok    bool
	calls atomic.Int32
}

func (c *countingLookup) AgentFor(_ context.Context, _ int64) (AgentConfig, bool, error) {
	c.calls.Add(1)
	return c.agent, c.ok, nil
}

func TestResolveForWrapsResilientAndCaches(t *testing.T) {
	lookup := &countingLookup{agent: AgentConfig{Vendor: "gemini", Model: "g"}, ok: true}
	r, err := NewCompanyResolver(lookup, Settings{Vendor: "gemini", GeminiKey: "k"})
	if err != nil {
		t.Fatalf("NewCompanyResolver err = %v", err)
	}
	r.WithResilient(ResilientOptions{Timeout: time.Second}).WithCacheTTL(time.Minute)

	p1, err := r.ResolveFor(context.Background(), 7)
	if err != nil {
		t.Fatalf("ResolveFor err = %v", err)
	}
	if _, ok := p1.(*resilient); !ok {
		t.Fatalf("provider type = %T, want *resilient", p1)
	}

	p2, err := r.ResolveFor(context.Background(), 7)
	if err != nil {
		t.Fatalf("ResolveFor (cached) err = %v", err)
	}
	if p1 != p2 {
		t.Fatal("ResolveFor should return the cached provider instance within TTL")
	}

	if _, err := r.AgentFor(context.Background(), 7); err != nil {
		t.Fatalf("AgentFor err = %v", err)
	}
	if got := lookup.calls.Load(); got != 1 {
		t.Fatalf("lookup calls = %d, want 1 (cached)", got)
	}
}

func TestResolveForRebuildsAfterTTLExpiry(t *testing.T) {
	lookup := &countingLookup{agent: AgentConfig{Vendor: "gemini", Model: "g"}, ok: true}
	r, err := NewCompanyResolver(lookup, Settings{Vendor: "gemini", GeminiKey: "k"})
	if err != nil {
		t.Fatalf("NewCompanyResolver err = %v", err)
	}
	r.WithCacheTTL(time.Nanosecond)

	if _, err := r.ResolveFor(context.Background(), 7); err != nil {
		t.Fatalf("ResolveFor err = %v", err)
	}
	time.Sleep(time.Millisecond)
	if _, err := r.ResolveFor(context.Background(), 7); err != nil {
		t.Fatalf("ResolveFor err = %v", err)
	}
	if got := lookup.calls.Load(); got < 2 {
		t.Fatalf("lookup calls = %d, want >= 2 after TTL expiry", got)
	}
}

func TestNewCompanyResolverFailureCases(t *testing.T) {
	cases := []struct {
		name     string
		defaults Settings
		wantSub  string
	}{
		{name: "missing_vendor", defaults: Settings{}, wantSub: "defaults.Vendor is required"},
		{name: "whitespace_vendor", defaults: Settings{Vendor: "   "}, wantSub: "defaults.Vendor is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewCompanyResolver(nil, tc.defaults)
			if err == nil {
				t.Fatalf("got nil err, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestResolveForFailureCases(t *testing.T) {
	cases := []struct {
		name      string
		companyID int64
		lookup    stubLookup
		defaults  Settings
		wantSub   string
	}{
		{
			name:      "rejects_zero_company",
			companyID: 0,
			defaults:  Settings{Vendor: "openai", OpenAIKey: "k"},
			wantSub:   "company_id must be positive",
		},
		{
			name:      "lookup_error_bubbles",
			companyID: 7,
			lookup:    stubLookup{err: errors.New("db gone")},
			defaults:  Settings{Vendor: "openai", OpenAIKey: "k"},
			wantSub:   "lookup company 7",
		},
		{
			name:      "agent_vendor_unsupported_propagates",
			companyID: 7,
			lookup:    stubLookup{ok: true, agent: AgentConfig{Vendor: "cohere"}},
			defaults:  Settings{Vendor: "openai", OpenAIKey: "k"},
			wantSub:   `unsupported vendor "cohere"`,
		},
		{
			name:      "agent_vendor_anthropic_but_no_key",
			companyID: 7,
			lookup:    stubLookup{ok: true, agent: AgentConfig{Vendor: "anthropic"}},
			defaults:  Settings{Vendor: "openai", OpenAIKey: "k"},
			wantSub:   "api key is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := NewCompanyResolver(tc.lookup, tc.defaults)
			if err != nil {
				t.Fatalf("NewCompanyResolver err = %v", err)
			}
			_, err = r.ResolveFor(context.Background(), tc.companyID)
			if err == nil {
				t.Fatalf("got nil err, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestResolveForDisabledWhenNoAgent(t *testing.T) {
	r, err := NewCompanyResolver(NoopAgentLookup{}, Settings{
		Vendor:    "openai",
		OpenAIKey: "k",
		Model:     "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("NewCompanyResolver err = %v", err)
	}
	p, err := r.ResolveFor(context.Background(), 7)
	if !errors.Is(err, ports.ErrAIDisabled) {
		t.Fatalf("ResolveFor err = %v, want ErrAIDisabled", err)
	}
	if p != nil {
		t.Fatalf("provider = %T, want nil when ai is disabled", p)
	}
}

func TestResolveForUsesAgentVendorAndModel(t *testing.T) {
	lookup := stubLookup{ok: true, agent: AgentConfig{Vendor: "gemini", Model: "gemini-2.0-flash-thinking-exp"}}
	r, err := NewCompanyResolver(lookup, Settings{
		Vendor:    "openai",
		OpenAIKey: "irrelevant",
		GeminiKey: "gk",
	})
	if err != nil {
		t.Fatalf("NewCompanyResolver err = %v", err)
	}
	p, err := r.ResolveFor(context.Background(), 7)
	if err != nil {
		t.Fatalf("ResolveFor err = %v", err)
	}
	if p == nil {
		t.Fatal("provider = nil")
	}
}

func TestApplyVendorKey(t *testing.T) {
	cases := []struct {
		vendor string
		field  func(Settings) string
	}{
		{"openai", func(s Settings) string { return s.OpenAIKey }},
		{"openrouter", func(s Settings) string { return s.OpenRouterKey }},
		{"anthropic", func(s Settings) string { return s.AnthropicKey }},
		{"claude", func(s Settings) string { return s.AnthropicKey }},
		{"gemini", func(s Settings) string { return s.GeminiKey }},
	}
	for _, tc := range cases {
		t.Run(tc.vendor, func(t *testing.T) {
			var s Settings
			applyVendorKey(&s, tc.vendor, "company-key")
			if got := tc.field(s); got != "company-key" {
				t.Fatalf("vendor %s key field = %q, want company-key", tc.vendor, got)
			}
		})
	}
}

func TestResolveForInjectsPerCompanyKey(t *testing.T) {
	lookup := stubLookup{ok: true, agent: AgentConfig{
		Vendor: "gemini",
		Model:  "gemini-2.5-flash",
		APIKey: "company-gemini-key",
	}}
	r, err := NewCompanyResolver(lookup, Settings{
		Vendor:    "openai",
		OpenAIKey: "platform-openai",
	})
	if err != nil {
		t.Fatalf("NewCompanyResolver err = %v", err)
	}
	if _, err := r.ResolveFor(context.Background(), 7); err != nil {
		t.Fatalf("ResolveFor should inject the per-company key so gemini resolves, got err = %v", err)
	}
}

func TestResolveForTreatsEmptyAgentVendorAsDefault(t *testing.T) {
	lookup := stubLookup{ok: true, agent: AgentConfig{Vendor: "", Model: "claude-haiku-4-5-20251001"}}
	r, err := NewCompanyResolver(lookup, Settings{
		Vendor:       "anthropic",
		AnthropicKey: "k",
	})
	if err != nil {
		t.Fatalf("NewCompanyResolver err = %v", err)
	}
	if _, err := r.ResolveFor(context.Background(), 7); err != nil {
		t.Fatalf("ResolveFor err = %v", err)
	}
}
