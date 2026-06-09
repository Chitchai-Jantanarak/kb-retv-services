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

func TestNewLLMGeneratorFailureCases(t *testing.T) {
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
			_, err := NewLLMGenerator(LLMGeneratorConfig{Registry: passed, Resolve: tc.resolve})
			if err == nil {
				t.Fatalf("err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want sub %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestGeneratorSkipsWhenNoCandidates(t *testing.T) {
	g, err := NewLLMGenerator(LLMGeneratorConfig{
		Registry: registry(t),
		Resolve: func(_ context.Context, _ int64) (ports.LLMProvider, error) {
			t.Fatal("provider must not be called for empty candidates")
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("NewLLMGenerator: %v", err)
	}
	draft, err := g.Generate(ctxkey.WithCompanyID(context.Background(), 1), Query{Text: "q"}, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if draft != "" {
		t.Fatalf("draft = %q, want empty", draft)
	}
}

func TestGeneratorSkipsWithoutCompanyID(t *testing.T) {
	g, err := NewLLMGenerator(LLMGeneratorConfig{
		Registry: registry(t),
		Resolve: func(_ context.Context, _ int64) (ports.LLMProvider, error) {
			t.Fatal("provider must not be called without company_id")
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("NewLLMGenerator: %v", err)
	}
	draft, err := g.Generate(context.Background(), Query{Text: "q"}, []Candidate{{ID: "a"}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if draft != "" {
		t.Fatalf("draft = %q, want empty (no company_id)", draft)
	}
}

func TestGeneratorReturnsProviderError(t *testing.T) {
	g, err := NewLLMGenerator(LLMGeneratorConfig{
		Registry: registry(t),
		Resolve: func(_ context.Context, _ int64) (ports.LLMProvider, error) {
			return fakeProvider{err: errors.New("503 upstream")}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewLLMGenerator: %v", err)
	}
	_, err = g.Generate(ctxkey.WithCompanyID(context.Background(), 1), Query{Text: "q"}, []Candidate{{ID: "a", Content: "x"}})
	if err == nil || !strings.Contains(err.Error(), "503 upstream") {
		t.Fatalf("err = %v, want upstream error wrapped", err)
	}
}

func TestGeneratorHappyPath(t *testing.T) {
	g, err := NewLLMGenerator(LLMGeneratorConfig{
		Registry: registry(t),
		Resolve: func(_ context.Context, _ int64) (ports.LLMProvider, error) {
			return fakeProvider{text: " Restart the printer and roll back firmware. [src:a] "}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewLLMGenerator: %v", err)
	}
	draft, err := g.Generate(ctxkey.WithCompanyID(context.Background(), 1), Query{Text: "printer offline"}, []Candidate{{ID: "a", Title: "Fix", Content: "restart"}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(draft, "Restart the printer") || !strings.Contains(draft, "[src:a]") {
		t.Fatalf("draft = %q", draft)
	}
}
