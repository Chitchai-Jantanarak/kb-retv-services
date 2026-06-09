package prompts

import (
	"sort"
	"strings"
	"testing"
)

func TestRenderFailureCases(t *testing.T) {
	cases := []struct {
		name    string
		tmpl    Template
		vars    map[string]string
		wantSub string
	}{
		{
			name:    "missing_single_var",
			tmpl:    Template{Name: "t", User: "{{q}}", RequiredVars: []string{"q"}},
			vars:    map[string]string{},
			wantSub: `template "t" missing required vars: q`,
		},
		{
			name:    "missing_multiple_vars_sorted",
			tmpl:    Template{Name: "t", User: "{{a}}/{{b}}", RequiredVars: []string{"b", "a"}},
			vars:    map[string]string{},
			wantSub: "a, b",
		},
		{
			name:    "whitespace_only_var_rejected",
			tmpl:    Template{Name: "t", User: "{{q}}", RequiredVars: []string{"q"}},
			vars:    map[string]string{"q": "   "},
			wantSub: "missing required vars: q",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.tmpl.Render(tc.vars)
			if err == nil {
				t.Fatalf("Render() err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestRenderSubstitutesAllVars(t *testing.T) {
	tmpl := Template{
		Name:         "t",
		System:       "hello {{name}}",
		User:         "Q: {{q}} - tone={{tone}}",
		RequiredVars: []string{"q", "tone", "name"},
		MaxToks:      10,
		Temp:         0.3,
	}
	p, err := tmpl.Render(map[string]string{"q": "hi", "tone": "warm", "name": "world"})
	if err != nil {
		t.Fatalf("Render() err = %v", err)
	}
	if p.System != "hello world" {
		t.Fatalf("System = %q", p.System)
	}
	if p.User != "Q: hi - tone=warm" {
		t.Fatalf("User = %q", p.User)
	}
	if p.MaxToks != 10 || p.Temp != 0.3 {
		t.Fatalf("MaxToks/Temp not propagated: %d/%f", p.MaxToks, p.Temp)
	}
}

func TestDefaultsCoverAllNamedTemplates(t *testing.T) {
	want := []string{
		NameRewrite,
		NameClassifyStage1,
		NameClassifyStage2,
		NameHyDE,
		NameFusion,
		NameRerank,
		NameCRAG,
		NameSelfRAG,
		NameGenerate,
	}
	got := make([]string, 0, len(Defaults()))
	for _, tmpl := range Defaults() {
		got = append(got, tmpl.Name)
	}
	sort.Strings(want)
	sort.Strings(got)
	if strings.Join(want, ",") != strings.Join(got, ",") {
		t.Fatalf("defaults mismatch:\nwant=%v\n got=%v", want, got)
	}
}

func TestEveryDefaultRequiredVarIsReferencedInBody(t *testing.T) {
	for _, tmpl := range Defaults() {
		body := tmpl.System + "\n" + tmpl.User
		for _, v := range tmpl.RequiredVars {
			needle := "{{" + v + "}}"
			if !strings.Contains(body, needle) {
				t.Errorf("template %q lists %q as required but body has no %s",
					tmpl.Name, v, needle)
			}
		}
	}
}

func TestRegistryGetFailureCases(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() err = %v", err)
	}
	if _, err := r.Get("nonexistent"); err == nil {
		t.Fatal("Get(nonexistent) err = nil, want unknown-template error")
	} else if !strings.Contains(err.Error(), `unknown template "nonexistent"`) {
		t.Fatalf("err = %q", err.Error())
	}
}

func TestRegistryGetReturnsTemplate(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() err = %v", err)
	}
	tmpl, err := r.Get(NameCRAG)
	if err != nil {
		t.Fatalf("Get(crag) err = %v", err)
	}
	if tmpl.Name != NameCRAG {
		t.Fatalf("Name = %q, want %q", tmpl.Name, NameCRAG)
	}
}

func TestRegistryRoundTripRenderForEveryDefault(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() err = %v", err)
	}
	// Vars sufficient to render any template in Defaults()
	vars := map[string]string{
		"query":            "printer not working",
		"report":           "Printer in branch HQ stopped responding after firmware update",
		"severity_options": "critical, high, medium, low",
		"symptom_options":  "printer_offline, paper_jam",
		"count":            "4",
		"candidates":       "1: foo\n2: bar",
		"evidence":         "[src:1] Restart printer\n[src:2] Roll back firmware",
		"draft":            "Try rolling back firmware",
		"tone":             "friendly",
		"max_sentences":    "3",
	}
	for _, name := range r.Names() {
		tmpl, err := r.Get(name)
		if err != nil {
			t.Fatalf("Get(%s) err = %v", name, err)
		}
		p, err := tmpl.Render(vars)
		if err != nil {
			t.Fatalf("Render(%s) err = %v", name, err)
		}
		if p.User == "" {
			t.Fatalf("template %s: rendered User is empty", name)
		}
		// Ensure no leftover unsubstituted placeholders for required vars
		for _, v := range tmpl.RequiredVars {
			placeholder := "{{" + v + "}}"
			if strings.Contains(p.System+p.User, placeholder) {
				t.Errorf("template %s: placeholder %s not substituted", name, placeholder)
			}
		}
	}
}
