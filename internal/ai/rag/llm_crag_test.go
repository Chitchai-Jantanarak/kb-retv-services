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

type fakeProvider struct {
	text string
	err  error
}

func (f fakeProvider) Generate(_ context.Context, _ ports.Prompt) (ports.Completion, error) {
	return ports.Completion{Text: f.text}, f.err
}

func (f fakeProvider) GenerateJSON(_ context.Context, _ ports.Prompt) (ports.Completion, error) {
	return ports.Completion{Text: f.text}, f.err
}

func (f fakeProvider) Stream(_ context.Context, _ ports.Prompt) (<-chan ports.Completion, error) {
	return nil, errors.New("not implemented")
}

func registry(t *testing.T) *prompts.Registry {
	t.Helper()
	r, err := prompts.NewRegistry()
	if err != nil {
		t.Fatalf("prompts.NewRegistry: %v", err)
	}
	return r
}

func TestNewLLMCRAGFailureCases(t *testing.T) {
	reg := registry(t)
	provider := func(_ context.Context, _ int64) (ports.LLMProvider, error) {
		return fakeProvider{}, nil
	}

	cases := []struct {
		name     string
		registry *prompts.Registry
		resolve  ProviderForCompany
		wantSub  string
	}{
		{name: "nil_registry", registry: nil, resolve: provider, wantSub: "registry is required"},
		{name: "nil_resolver", registry: reg, resolve: nil, wantSub: "resolver is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewLLMCRAG(tc.registry, tc.resolve)
			if err == nil {
				t.Fatalf("err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want sub %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestLLMCRAGGradeEmptyContent(t *testing.T) {
	reg := registry(t)
	c, err := NewLLMCRAG(reg, func(_ context.Context, _ int64) (ports.LLMProvider, error) {
		t.Fatal("provider must not be called when content is empty")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("NewLLMCRAG: %v", err)
	}
	result, err := c.Grade(ctxkey.WithCompanyID(context.Background(), 1), "q", "")
	if err != nil {
		t.Fatalf("Grade: %v", err)
	}
	if result.Verdict != VerdictIrrelevant {
		t.Fatalf("verdict = %v, want VerdictIrrelevant", result.Verdict)
	}
}

func TestLLMCRAGGradeMissingCompanyID(t *testing.T) {
	reg := registry(t)
	c, err := NewLLMCRAG(reg, func(_ context.Context, _ int64) (ports.LLMProvider, error) {
		t.Fatal("provider must not be called without company_id")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("NewLLMCRAG: %v", err)
	}
	_, err = c.Grade(context.Background(), "q", "evidence")
	if err == nil {
		t.Fatal("Grade err = nil, want error when company_id is missing (no silent promotion)")
	}
	if !strings.Contains(err.Error(), "company_id missing") {
		t.Fatalf("err = %q, want company_id missing", err.Error())
	}
}

func TestLLMCRAGGradeFailureCases(t *testing.T) {
	reg := registry(t)
	cases := []struct {
		name     string
		provider fakeProvider
		wantSub  string
	}{
		{name: "provider_error", provider: fakeProvider{err: errors.New("upstream 500")}, wantSub: "upstream 500"},
		{name: "invalid_json", provider: fakeProvider{text: "not-json"}, wantSub: "parse"},
		{name: "unknown_verdict", provider: fakeProvider{text: `{"verdict":"maybe","confidence":0.5}`}, wantSub: "unknown verdict"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewLLMCRAG(reg, func(_ context.Context, _ int64) (ports.LLMProvider, error) {
				return tc.provider, nil
			})
			if err != nil {
				t.Fatalf("NewLLMCRAG: %v", err)
			}
			_, err = c.Grade(ctxkey.WithCompanyID(context.Background(), 5), "q", "e")
			if err == nil {
				t.Fatalf("err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want sub %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestLLMCRAGGradeHappyPath(t *testing.T) {
	reg := registry(t)
	cases := []struct {
		name string
		text string
		want CRAGResult
	}{
		{name: "relevant", text: `{"verdict":"relevant","confidence":0.9,"missing":""}`, want: CRAGResult{Verdict: VerdictRelevant, Confidence: 0.9, Graded: true}},
		{name: "partial", text: `{"verdict":"PARTIAL","confidence":0.5,"missing":"x"}`, want: CRAGResult{Verdict: VerdictPartial, Confidence: 0.5, Missing: "x", Graded: true}},
		{name: "irrelevant", text: `{"verdict":"irrelevant","confidence":0.1,"missing":"y"}`, want: CRAGResult{Verdict: VerdictIrrelevant, Confidence: 0.1, Missing: "y", Graded: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewLLMCRAG(reg, func(_ context.Context, _ int64) (ports.LLMProvider, error) {
				return fakeProvider{text: tc.text}, nil
			})
			if err != nil {
				t.Fatalf("NewLLMCRAG: %v", err)
			}
			got, err := c.Grade(ctxkey.WithCompanyID(context.Background(), 1), "q", "evidence")
			if err != nil {
				t.Fatalf("Grade: %v", err)
			}
			if got != tc.want {
				t.Fatalf("result = %+v, want %+v", got, tc.want)
			}
		})
	}
}
