package prompts

import "testing"

func TestThinkingDirectiveParsed(t *testing.T) {
	raw := `@name      probe
@required  q
@max_toks  200
@thinking  0

@system
sys

@user
{{q}}
`
	tmpl, err := parsePrompt(raw)
	if err != nil {
		t.Fatalf("parseTemplate: %v", err)
	}
	if tmpl.ThinkBudget == nil {
		t.Fatal("ThinkBudget = nil, want an explicit 0 so the provider disables thinking")
	}
	if *tmpl.ThinkBudget != 0 {
		t.Fatalf("ThinkBudget = %d, want 0", *tmpl.ThinkBudget)
	}

	p, err := tmpl.Render(map[string]string{"q": "hi"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if p.ThinkBudget == nil || *p.ThinkBudget != 0 {
		t.Fatalf("Prompt.ThinkBudget = %v, want the template value carried through", p.ThinkBudget)
	}
}

func TestThinkingAbsentLeavesProviderDefault(t *testing.T) {
	raw := `@name      probe2
@max_toks  200

@system
sys

@user
hi
`
	tmpl, err := parsePrompt(raw)
	if err != nil {
		t.Fatalf("parseTemplate: %v", err)
	}
	if tmpl.ThinkBudget != nil {
		t.Fatalf("ThinkBudget = %v, want nil so the provider default is untouched", *tmpl.ThinkBudget)
	}
}
