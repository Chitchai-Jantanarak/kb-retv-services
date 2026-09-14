// Package instructions turns a tenant admin's free-text description of how
// their AI assistant should behave into a fuller, structured instruction
// (stored by Laravel as ai_agents.system_prompt and rendered inside the chat
// workflow's <company_instructions> fence).
package instructions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/ai/rag"
	apperr "github.com/my/app/internal/shared/errors"
)

const (
	maxTextRunes = 2000
	maxQuestions = 3
	maxChanges   = 12

	// maxAnswerRunes bounds each clarifying question and its answer, so the
	// answers slot cannot smuggle past the limit that applies to the text.
	maxAnswerRunes = 500

	localeEnglish = "en"
)

// Answer is a previously asked clarifying question and the admin's reply to
// it, so a follow-up enhance call can fold the answer in instead of asking
// the same question twice.
type Answer struct {
	Question string
	Answer   string
}

// Request is the admin-supplied input to Enhance.
type Request struct {
	Text    string
	Locale  string
	Answers []Answer
}

// Change describes one edit the model made between the admin's text and the
// enhanced instruction.
type Change struct {
	Kind string // "rewritten", "added", or "removed"
	Text string
	Why  string
}

// Result is the enhanced instruction plus what changed and what's unclear.
type Result struct {
	Enhanced  string
	Changes   []Change
	Questions []string
}

// Enhancer rewrites operator instructions via an LLM call.
type Enhancer struct {
	tmpl    prompts.Template
	resolve rag.ProviderForCompany
}

// New builds an Enhancer from the shared prompt registry and a per-company
// provider resolver (see rag.ProviderForCompany / llm.CompanyResolver.ForTask).
func New(registry *prompts.Registry, resolve rag.ProviderForCompany) (*Enhancer, error) {
	if registry == nil {
		return nil, fmt.Errorf("instructions: prompt registry is required")
	}
	if resolve == nil {
		return nil, fmt.Errorf("instructions: provider resolver is required")
	}
	tmpl, err := registry.Get(prompts.NameInstructionsEnhance)
	if err != nil {
		return nil, fmt.Errorf("instructions: %w", err)
	}
	return &Enhancer{tmpl: tmpl, resolve: resolve}, nil
}

// Enhance validates req, asks the model to rewrite req.Text, and returns the
// rewritten instruction along with what changed and up to three clarifying
// questions.
func (e *Enhancer) Enhance(ctx context.Context, companyID int64, req Request) (Result, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return Result{}, apperr.New(apperr.CodeInvalidInput, "text is required")
	}
	if n := len([]rune(text)); n > maxTextRunes {
		return Result{}, apperr.New(apperr.CodeInvalidInput, fmt.Sprintf("text must be %d characters or fewer", maxTextRunes))
	}

	prompt, err := e.tmpl.Render(map[string]string{
		"language": promptLanguage(text, req.Locale),
		"text":     text,
		"answers":  renderAnswers(req.Answers),
	})
	if err != nil {
		return Result{}, fmt.Errorf("instructions: render prompt: %w", err)
	}

	provider, err := e.resolve(ctx, companyID)
	if err != nil {
		return Result{}, err
	}

	completion, err := provider.GenerateJSON(ctx, prompt)
	if err != nil {
		return Result{}, err
	}

	var parsed struct {
		Enhanced enhancedText `json:"enhanced"`
		Changes  []struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
			Why  string `json:"why"`
		} `json:"changes"`
		Questions []string `json:"questions"`
	}
	if err := json.Unmarshal([]byte(rag.ExtractJSONObject(completion.Text)), &parsed); err != nil {
		return Result{}, fmt.Errorf("instructions: parse %q: %w", completion.Text, err)
	}

	enhanced := strings.TrimSpace(string(parsed.Enhanced))
	if enhanced == "" {
		return Result{}, fmt.Errorf("instructions: model returned no enhanced text")
	}
	if n := len([]rune(enhanced)); n > maxTextRunes {
		return Result{}, fmt.Errorf("instructions: model returned %d characters, over the %d limit", n, maxTextRunes)
	}

	result := Result{Enhanced: enhanced}
	for _, c := range parsed.Changes {
		kind := strings.ToLower(strings.TrimSpace(c.Kind))
		if kind != "rewritten" && kind != "added" && kind != "removed" {
			continue
		}
		result.Changes = append(result.Changes, Change{
			Kind: kind,
			Text: strings.TrimSpace(c.Text),
			Why:  strings.TrimSpace(c.Why),
		})
		if len(result.Changes) >= maxChanges {
			break
		}
	}
	for _, q := range parsed.Questions {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		result.Questions = append(result.Questions, q)
		if len(result.Questions) >= maxQuestions {
			break
		}
	}
	return result, nil
}

// renderAnswers formats previously answered clarifying questions for the
// template's {{answers}} slot. It returns "" when there is nothing to fold
// in, so the template reads fine with no leftover header line.
func renderAnswers(answers []Answer) string {
	var lines []string
	for _, a := range answers {
		if strings.TrimSpace(a.Answer) == "" {
			continue
		}
		lines = append(lines, "Q: "+clip(a.Question, maxAnswerRunes)+"\nA: "+clip(a.Answer, maxAnswerRunes))
		if len(lines) >= maxQuestions {
			break
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "The admin answered earlier questions; fold the answers in and do not ask them again:\n" + strings.Join(lines, "\n") + "\n\n"
}

// promptLanguage names the language the whole reply must be written in: the
// language the admin's text is in, judged by script, falling back to the UI
// locale only when the text carries no letters at all.
func promptLanguage(text, locale string) string {
	thai, latin := 0, 0
	for _, r := range text {
		switch {
		case r >= 0x0E00 && r <= 0x0E7F:
			thai++
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			latin++
		}
	}
	switch {
	case thai > latin:
		return "Thai"
	case latin > thai:
		return "English"
	case locale == localeEnglish:
		return "English"
	default:
		return "Thai"
	}
}

// enhancedText accepts the "enhanced" field as the string the prompt asks for
// or, when the model ignores that and returns an object keyed by heading, as
// that object folded back into headed text in the canonical order.
type enhancedText string

var enhancedHeadings = []string{"Role", "Instructions", "Steps", "End goal", "Stay within"}

func (e *enhancedText) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*e = enhancedText(s)
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return fmt.Errorf("enhanced: want string or object, got %s", strings.TrimSpace(string(b)))
	}
	var out []string
	take := func(k string) {
		raw, ok := m[k]
		if !ok {
			return
		}
		delete(m, k)
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			var items []string
			if json.Unmarshal(raw, &items) != nil {
				return
			}
			v = strings.Join(items, "\n")
		}
		if strings.TrimSpace(v) == "" {
			return
		}
		out = append(out, k+"\n"+strings.TrimSpace(v))
	}
	for _, k := range enhancedHeadings {
		take(k)
	}
	for k := range m {
		take(k)
	}
	*e = enhancedText(strings.Join(out, "\n\n"))
	return nil
}

// clip trims s and cuts it to at most n runes.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
