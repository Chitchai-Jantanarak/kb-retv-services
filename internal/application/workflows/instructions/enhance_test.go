package instructions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/domain/ports"
	apperr "github.com/my/app/internal/shared/errors"
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

func TestEnhanceHappyPath(t *testing.T) {
	reg := registry(t)
	const fenced = "```json\n" +
		`{"enhanced":"Role\nSupport agent for Acme.","changes":[` +
		`{"kind":"removed","text":"promised a full refund","why":"pricing commitments are not allowed"},` +
		`{"kind":"ADDED ","text":"Stay within section","why":"structure"}],` +
		`"questions":["What are your support hours?","Which products do you sell?","Do you handle billing questions?","This one should be dropped"]}` +
		"\n```"

	e, err := New(reg, func(_ context.Context, _ int64) (ports.LLMProvider, error) {
		return fakeProvider{text: fenced}, nil
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := e.Enhance(context.Background(), 1, Request{Text: "be a helpful support agent for Acme and promise refunds"})
	if err != nil {
		t.Fatalf("Enhance: %v", err)
	}

	if result.Enhanced != "Role\nSupport agent for Acme." {
		t.Fatalf("Enhanced = %q", result.Enhanced)
	}
	if len(result.Changes) != 2 {
		t.Fatalf("Changes = %+v, want 2 entries", result.Changes)
	}
	if result.Changes[0].Kind != "removed" {
		t.Fatalf("Changes[0].Kind = %q, want removed", result.Changes[0].Kind)
	}
	if result.Changes[1].Kind != "added" {
		t.Fatalf("Changes[1].Kind = %q, want added (lowercased)", result.Changes[1].Kind)
	}
	if len(result.Questions) != 3 {
		t.Fatalf("Questions = %v, want capped at 3", result.Questions)
	}
}

func TestEnhanceEmptyTextIsInvalidInput(t *testing.T) {
	reg := registry(t)
	e, err := New(reg, func(_ context.Context, _ int64) (ports.LLMProvider, error) {
		t.Fatal("provider must not be called for empty text")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = e.Enhance(context.Background(), 1, Request{Text: "   "})
	if err == nil {
		t.Fatal("Enhance err = nil, want invalid_input error")
	}
	ae, ok := apperr.As(err)
	if !ok {
		t.Fatalf("err = %v, want *apperr.AppError", err)
	}
	if ae.Code != apperr.CodeInvalidInput {
		t.Fatalf("code = %q, want %q", ae.Code, apperr.CodeInvalidInput)
	}
}

func TestEnhanceTextTooLongIsInvalidInput(t *testing.T) {
	reg := registry(t)
	e, err := New(reg, func(_ context.Context, _ int64) (ports.LLMProvider, error) {
		t.Fatal("provider must not be called for oversized text")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = e.Enhance(context.Background(), 1, Request{Text: strings.Repeat("a", 2001)})
	if err == nil {
		t.Fatal("Enhance err = nil, want invalid_input error")
	}
	ae, ok := apperr.As(err)
	if !ok || ae.Code != apperr.CodeInvalidInput {
		t.Fatalf("err = %v, want invalid_input", err)
	}
}

func TestEnhanceProviderErrorPropagates(t *testing.T) {
	reg := registry(t)
	upstream := errors.New("upstream 500")
	e, err := New(reg, func(_ context.Context, _ int64) (ports.LLMProvider, error) {
		return fakeProvider{err: upstream}, nil
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = e.Enhance(context.Background(), 1, Request{Text: "hello"})
	if err == nil || !strings.Contains(err.Error(), "upstream 500") {
		t.Fatalf("err = %v, want upstream 500", err)
	}
}

func TestNewFailureCases(t *testing.T) {
	reg := registry(t)
	provider := func(_ context.Context, _ int64) (ports.LLMProvider, error) {
		return fakeProvider{}, nil
	}

	cases := []struct {
		name     string
		registry *prompts.Registry
		resolve  func(context.Context, int64) (ports.LLMProvider, error)
		wantSub  string
	}{
		{name: "nil_registry", registry: nil, resolve: provider, wantSub: "registry is required"},
		{name: "nil_resolver", registry: reg, resolve: nil, wantSub: "resolver is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.registry, tc.resolve)
			if err == nil {
				t.Fatalf("err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want sub %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestEnhancedTextAcceptsObjectShape(t *testing.T) {
	var got enhancedText
	if err := json.Unmarshal([]byte(`{"Stay within":"x","Role":"r","Instructions":["- a","- b"]}`), &got); err != nil {
		t.Fatal(err)
	}
	if string(got) != "Role\nr\n\nInstructions\n- a\n- b\n\nStay within\nx" {
		t.Fatalf("got %q", got)
	}
	if err := json.Unmarshal([]byte(`"plain"`), &got); err != nil || got != "plain" {
		t.Fatalf("string form: %q %v", got, err)
	}
}

func TestPromptLanguageFollowsTheTextScript(t *testing.T) {
	if got := promptLanguage("We service Pudu robots in Thailand", "th"); got != "English" {
		t.Fatalf("english text under th locale = %q", got)
	}
	if got := promptLanguage("เราดูแลหุ่นยนต์ Pudu", "en"); got != "Thai" {
		t.Fatalf("thai text under en locale = %q", got)
	}
	if got := promptLanguage("1234", "en"); got != "English" {
		t.Fatalf("no letters falls back to locale = %q", got)
	}
}

func TestRenderAnswersCapsCountAndLength(t *testing.T) {
	long := strings.Repeat("ก", maxAnswerRunes+50)
	answers := []Answer{
		{Question: "q1", Answer: long},
		{Question: "q2", Answer: "a2"},
		{Question: "q3", Answer: "a3"},
		{Question: "q4", Answer: "a4"},
	}
	got := renderAnswers(answers)
	if strings.Contains(got, "q4") {
		t.Fatalf("more than %d answers rendered: %q", maxQuestions, got)
	}
	if n := len([]rune(got)); n > maxAnswerRunes*2*maxQuestions+400 {
		t.Fatalf("rendered answers unbounded at %d runes", n)
	}
	if strings.Count(got, "ก") > maxAnswerRunes {
		t.Fatalf("long answer not clipped")
	}
}
