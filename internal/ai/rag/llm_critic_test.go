package rag

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

func TestNewLLMCriticFailureCases(t *testing.T) {
	reg := registry(t)
	dummyResolve := func(_ context.Context, _ int64) (ports.LLMProvider, error) { return nil, nil }
	cases := []struct {
		name        string
		useRegistry bool
		resolve     ProviderForCompany
		wantSub     string
	}{
		{name: "nil_registry", useRegistry: false, resolve: dummyResolve, wantSub: "registry is required"},
		{name: "nil_resolver", useRegistry: true, resolve: nil, wantSub: "resolver is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var passed *prompts.Registry
			if tc.useRegistry {
				passed = reg
			}
			_, err := NewLLMCritic(passed, tc.resolve)
			if err == nil {
				t.Fatalf("err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want sub %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestCriticRejectsEmptyDraft(t *testing.T) {
	c, err := NewLLMCritic(registry(t), func(_ context.Context, _ int64) (ports.LLMProvider, error) {
		t.Fatal("provider must not be called for empty draft")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("NewLLMCritic: %v", err)
	}
	res, err := c.Critique(ctxkey.WithCompanyID(context.Background(), 1), Query{Text: "q"}, "   ", nil)
	if err != nil {
		t.Fatalf("Critique: %v", err)
	}
	if res.Send {
		t.Fatalf("Send = true, want false on empty draft")
	}
}

func TestCriticPassesThroughWithoutCompanyID(t *testing.T) {
	c, err := NewLLMCritic(registry(t), func(_ context.Context, _ int64) (ports.LLMProvider, error) {
		t.Fatal("provider must not be called without company_id")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("NewLLMCritic: %v", err)
	}
	res, err := c.Critique(context.Background(), Query{Text: "q"}, "draft", nil)
	if err != nil {
		t.Fatalf("Critique: %v", err)
	}
	if !res.Send {
		t.Fatalf("Send = false, want true (passthrough)")
	}
}

func TestCriticReturnsProviderError(t *testing.T) {
	c, err := NewLLMCritic(registry(t), func(_ context.Context, _ int64) (ports.LLMProvider, error) {
		return fakeProvider{err: errors.New("upstream 500")}, nil
	})
	if err != nil {
		t.Fatalf("NewLLMCritic: %v", err)
	}
	_, err = c.Critique(ctxkey.WithCompanyID(context.Background(), 1), Query{Text: "q"}, "draft", []Candidate{{ID: "a"}})
	if err == nil || !strings.Contains(err.Error(), "upstream 500") {
		t.Fatalf("err = %v", err)
	}
}

func TestCriticReturnsParseError(t *testing.T) {
	c, err := NewLLMCritic(registry(t), func(_ context.Context, _ int64) (ports.LLMProvider, error) {
		return fakeProvider{text: "not-json"}, nil
	})
	if err != nil {
		t.Fatalf("NewLLMCritic: %v", err)
	}
	_, err = c.Critique(ctxkey.WithCompanyID(context.Background(), 1), Query{Text: "q"}, "draft", []Candidate{{ID: "a"}})
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("err = %v", err)
	}
}

func TestCriticHappyPaths(t *testing.T) {
	cases := []struct {
		name        string
		text        string
		wantSupport bool
		wantSend    bool
	}{
		{name: "send_supported", text: `{"supported":true,"hallucinations":[],"refusal_reason":"","send":true}`, wantSupport: true, wantSend: true},
		{name: "hold_unsupported", text: `{"supported":false,"hallucinations":["claim X"],"refusal_reason":"no evidence","send":false}`, wantSupport: false, wantSend: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewLLMCritic(registry(t), func(_ context.Context, _ int64) (ports.LLMProvider, error) {
				return fakeProvider{text: tc.text}, nil
			})
			if err != nil {
				t.Fatalf("NewLLMCritic: %v", err)
			}
			res, err := c.Critique(ctxkey.WithCompanyID(context.Background(), 1), Query{Text: "q"}, "draft", []Candidate{{ID: "a", Content: "x"}})
			if err != nil {
				t.Fatalf("Critique: %v", err)
			}
			if res.Supported != tc.wantSupport || res.Send != tc.wantSend {
				t.Fatalf("res = %+v, want supported=%v send=%v", res, tc.wantSupport, tc.wantSend)
			}
		})
	}
}
