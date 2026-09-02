package perms

import "testing"

func TestValid(t *testing.T) {
	t.Parallel()
	if !Valid("report.view") {
		t.Fatal("report.view must be valid")
	}
	if Valid("ai:reply:create") {
		t.Fatal("ai:reply:create is not a real key")
	}
}

func TestSetCoversKeys(t *testing.T) {
	t.Parallel()
	set := Set()
	if !set["portal.view"] || !set["kb.search"] {
		t.Fatalf("set missing known keys: %v", set)
	}
	if len(set) != len(Keys()) {
		t.Fatalf("set size %d != keys %d", len(set), len(Keys()))
	}
}

func TestCan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		granted  []string
		required string
		want     bool
	}{
		{"exact match", []string{"report.view"}, "report.view", true},
		{"no match", []string{"report.view"}, "report.create", false},
		{"star grants all", []string{"*"}, "handoff.invoke", true},
		{"prefix wildcard", []string{"report.*"}, "report.view", true},
		{"prefix wildcard nested", []string{"report.*"}, "report.view.own", true},
		{"prefix wildcard miss", []string{"report.*"}, "case.status.read", false},
		{"empty required allowed", []string{}, "", true},
		{"empty grant denied", []string{}, "report.view", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Can(tt.granted, tt.required); got != tt.want {
				t.Fatalf("Can(%v, %q) = %v, want %v", tt.granted, tt.required, got, tt.want)
			}
		})
	}
}
