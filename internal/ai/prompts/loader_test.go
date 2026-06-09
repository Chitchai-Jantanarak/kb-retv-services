package prompts

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestParsePromptFailureCases(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantSub string
	}{
		{
			name:    "missing_name_directive",
			src:     "@user\nhi {{q}}\n",
			wantSub: "missing @name",
		},
		{
			name:    "missing_user_body",
			src:     "@name x\n@required q\n",
			wantSub: "missing @user body",
		},
		{
			name:    "unknown_directive",
			src:     "@name x\n@foo bar\n@user\nhi\n",
			wantSub: "unknown directive @foo",
		},
		{
			name:    "bad_max_toks",
			src:     "@name x\n@max_toks notanumber\n@user\nhi\n",
			wantSub: "max_toks",
		},
		{
			name:    "bad_temp",
			src:     "@name x\n@temp NaNish\n@user\nhi\n",
			wantSub: "temp",
		},
		{
			name:    "unexpected_line_before_sections",
			src:     "just text without directive\n@name x\n@user\nhi\n",
			wantSub: "unexpected line before sections",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePrompt(tc.src)
			if err == nil {
				t.Fatalf("parsePrompt() err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestParsePromptHappyPath(t *testing.T) {
	src := `# comment line is fine before sections
@name      example
@required  q, ctx
@max_toks  256
@temp      0.7
@stop      \n\n
@system
You are precise.
Stay grounded.
@user
Q: {{q}}

Context: {{ctx}}
`
	tmpl, err := parsePrompt(src)
	if err != nil {
		t.Fatalf("parsePrompt() err = %v", err)
	}
	if tmpl.Name != "example" {
		t.Fatalf("Name = %q", tmpl.Name)
	}
	if len(tmpl.RequiredVars) != 2 || tmpl.RequiredVars[0] != "q" || tmpl.RequiredVars[1] != "ctx" {
		t.Fatalf("RequiredVars = %v", tmpl.RequiredVars)
	}
	if tmpl.MaxToks != 256 {
		t.Fatalf("MaxToks = %d", tmpl.MaxToks)
	}
	if tmpl.Temp != 0.7 {
		t.Fatalf("Temp = %f", tmpl.Temp)
	}
	if !strings.Contains(tmpl.System, "Stay grounded.") {
		t.Fatalf("System body lost newlines: %q", tmpl.System)
	}
	if !strings.Contains(tmpl.User, "Context: {{ctx}}") {
		t.Fatalf("User body wrong: %q", tmpl.User)
	}
}

func TestLoadFSRoundTripRendersAllCanonical(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() err = %v", err)
	}
	for _, name := range canonicalNames() {
		if _, err := r.Get(name); err != nil {
			t.Fatalf("missing canonical template %q: %v", name, err)
		}
	}
}

func TestNewRegistryFromFSOverridesEmbedded(t *testing.T) {
	overlay := fstest.MapFS{}
	for _, name := range canonicalNames() {
		body := "@name " + name + "\n@required q\n@system\noverride\n@user\noverride: {{q}}\n"
		overlay["overrides/"+name+".prompt"] = &fstest.MapFile{Data: []byte(body)}
	}
	r, err := NewRegistryFromFS(overlay, "overrides")
	if err != nil {
		t.Fatalf("NewRegistryFromFS() err = %v", err)
	}
	tmpl, err := r.Get(NameCRAG)
	if err != nil {
		t.Fatalf("Get(crag) err = %v", err)
	}
	if tmpl.System != "override" {
		t.Fatalf("overlay did not take effect: System = %q", tmpl.System)
	}
}

func TestNewRegistryFromFSFailsOnMissingCanonicalTemplate(t *testing.T) {
	overlay := fstest.MapFS{
		"x/rewrite.prompt": &fstest.MapFile{Data: []byte("@name rewrite\n@required query\n@user\nq: {{query}}\n")},
	}
	_, err := NewRegistryFromFS(overlay, "x")
	if err == nil {
		t.Fatal("err = nil, want missing-canonical error")
	}
	if !strings.Contains(err.Error(), "missing canonical templates") {
		t.Fatalf("err = %q", err.Error())
	}
}

func TestNewRegistryFromFSFailsOnDuplicateName(t *testing.T) {
	overlay := fstest.MapFS{
		"x/a.prompt": &fstest.MapFile{Data: []byte("@name dup\n@required q\n@user\nq: {{q}}\n")},
		"x/b.prompt": &fstest.MapFile{Data: []byte("@name dup\n@required q\n@user\nq: {{q}}\n")},
	}
	_, err := NewRegistryFromFS(overlay, "x")
	if err == nil {
		t.Fatal("err = nil, want duplicate-name error")
	}
	if !strings.Contains(err.Error(), "duplicate template") {
		t.Fatalf("err = %q", err.Error())
	}
}
