package prompts

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestParsePromptZoneEnforcement(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr string // empty means expect success
	}{
		{
			name: "tenant_var_before_locked_ok",
			src: "@name x\n@required q\n@tenant instructions\n@system\n" +
				"before {{instructions}}\n@locked\nafter, absolute.\n@user\nq: {{q}}\n",
		},
		{
			name: "tenant_var_after_locked_fails",
			src: "@name x\n@required q\n@tenant instructions\n@system\n" +
				"before\n@locked\nafter {{instructions}}\n@user\nq: {{q}}\n",
			wantErr: "after the @locked boundary",
		},
		{
			name: "tenant_without_locked_fails",
			src: "@name x\n@required q\n@tenant instructions\n@system\n" +
				"body {{instructions}}\n@user\nq: {{q}}\n",
			wantErr: "has no @locked boundary",
		},
		{
			name: "locked_without_tenant_fails",
			src: "@name x\n@required q\n@system\n" +
				"body\n@locked\nafter\n@user\nq: {{q}}\n",
			wantErr: "@locked without @tenant",
		},
		{
			name: "tenant_var_referenced_nowhere_fails",
			src: "@name x\n@required q\n@tenant foo\n@system\n" +
				"body\n@locked\nafter\n@user\nq: {{q}}\n",
			wantErr: "appears nowhere",
		},
		{
			name: "locked_inside_user_fails",
			src: "@name x\n@required q\n@tenant instructions\n@system\n" +
				"body {{instructions}}\n@user\nq: {{q}}\n@locked\nmore\n",
			wantErr: "@locked is only valid inside @system",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePrompt(tc.src)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("parsePrompt() err = %v, want success", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("parsePrompt() err = nil, want substring %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestLockedMarkerStrippedFromSystemBody(t *testing.T) {
	src := "@name x\n@required q\n@tenant instructions\n@system\n" +
		"before {{instructions}}\n@locked\nafter, absolute.\n@user\nq: {{q}}\n"
	tmpl, err := parsePrompt(src)
	if err != nil {
		t.Fatalf("parsePrompt() err = %v", err)
	}
	if strings.Contains(tmpl.System, "@locked") {
		t.Fatalf("@locked marker leaked into System: %q", tmpl.System)
	}
	if !strings.Contains(tmpl.System, "before {{instructions}}") {
		t.Fatalf("expected pre-locked content, got: %q", tmpl.System)
	}
	if !strings.Contains(tmpl.System, "after, absolute.") {
		t.Fatalf("expected post-locked content, got: %q", tmpl.System)
	}
	want := "before {{instructions}}\nafter, absolute."
	if tmpl.System != want {
		t.Fatalf("System = %q, want %q", tmpl.System, want)
	}
}

func TestLoadFSZoneValidation(t *testing.T) {
	fsys := fstest.MapFS{
		"templates/bad.prompt": &fstest.MapFile{Data: []byte(
			"@name bad\n@required q\n@tenant instructions\n@system\n" +
				"before\n@locked\nafter {{instructions}}\n@user\nq: {{q}}\n",
		)},
	}
	if _, err := LoadFS(fsys, "templates"); err == nil {
		t.Fatal("LoadFS() err = nil, want zone validation failure")
	} else if !strings.Contains(err.Error(), "after the @locked boundary") {
		t.Fatalf("err = %q", err.Error())
	}
}

func TestLoadEmbeddedPassesZoneValidation(t *testing.T) {
	if _, err := LoadEmbedded(); err != nil {
		t.Fatalf("LoadEmbedded() err = %v, want nil (shipped templates must satisfy @tenant/@locked contract)", err)
	}
}

func TestLockedZoneRejectsUndeclaredVariable(t *testing.T) {
	src := "@name x\n@tenant instructions\n@system\nbefore {{instructions}}\n@locked\nabsolute {{sneaky}}\n@user\nu\n"
	fsys := fstest.MapFS{"templates/x.prompt": &fstest.MapFile{Data: []byte(src)}}

	_, err := LoadFS(fsys, "templates")
	if err == nil {
		t.Fatal("expected an error for an undeclared variable inside the locked zone")
	}
	if !strings.Contains(err.Error(), "must be static text") {
		t.Fatalf("unexpected error: %v", err)
	}
}
