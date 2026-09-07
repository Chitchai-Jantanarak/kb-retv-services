package chat

import (
	"strings"
	"testing"

	"github.com/my/app/internal/ai/prompts"
)

// TestClarifyAndToolSummaryCarryInstructions guards the Task 2 wiring: the
// clarify and tool_summary templates must accept a non-empty "instructions"
// var and render it into the system prompt inside the <company_instructions>
// fence, the same way chat/chat_stream already do.
func TestClarifyAndToolSummaryCarryInstructions(t *testing.T) {
	r, err := prompts.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() err = %v", err)
	}

	block := instructionSection("Always greet the customer by first name.")
	if block == "" {
		t.Fatal("instructionSection() returned empty for non-empty input")
	}

	cases := []struct {
		name string
		vars map[string]string
	}{
		{
			name: prompts.NameClarify,
			vars: map[string]string{
				"language":     "English",
				"question":     "what's the status of my case?",
				"missing":      "case_code",
				"have":         "",
				"instructions": block,
			},
		},
		{
			name: prompts.NameToolSummary,
			vars: map[string]string{
				"language":     "English",
				"question":     "how many cases are open?",
				"count":        "2",
				"rows":         "REP-1|open\nREP-2|open",
				"instructions": block,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpl, err := r.Get(tc.name)
			if err != nil {
				t.Fatalf("Get(%s) err = %v", tc.name, err)
			}
			p, err := tmpl.Render(tc.vars)
			if err != nil {
				t.Fatalf("Render(%s) err = %v", tc.name, err)
			}
			if !strings.Contains(p.System, "<company_instructions>") {
				t.Fatalf("%s: rendered system prompt missing <company_instructions>: %q", tc.name, p.System)
			}
			if !strings.Contains(p.System, "Always greet the customer by first name.") {
				t.Fatalf("%s: rendered system prompt lost instruction text: %q", tc.name, p.System)
			}
		})
	}
}
