package tools

import (
	"testing"
	"testing/fstest"
)

func validPermsFixture() map[string]bool {
	return map[string]bool{"report.view": true, "kb.search": true}
}

func TestLoadValidTools(t *testing.T) {
	fsys := fstest.MapFS{
		"f7.json": {Data: []byte(`{"id":"f7_knowledge","intent":"kb_search","kind":"retrieval","handler":"knowledge","rbac":{"requires_permission":"kb.search"}}`)},
		"f1.json": {Data: []byte(`{"id":"f1_find_cases","intent":"find_cases","kind":"read","handler":"reports.find","rbac":{"requires_permission":"report.view"}}`)},
	}

	got, err := Load(fsys, validPermsFixture())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 tools, got %d", len(got))
	}
	if got[0].ID != "f1_find_cases" || got[1].ID != "f7_knowledge" {
		t.Fatalf("tools not sorted by id: %s, %s", got[0].ID, got[1].ID)
	}
}

func TestLoadRejectsUnknownPermission(t *testing.T) {
	fsys := fstest.MapFS{
		"bad.json": {Data: []byte(`{"id":"x","intent":"x","kind":"read","handler":"h","rbac":{"requires_permission":"report.explode"}}`)},
	}

	_, err := Load(fsys, validPermsFixture())
	if err == nil {
		t.Fatal("expected error for unknown permission")
	}
}

func TestLoadRejectsBadKind(t *testing.T) {
	fsys := fstest.MapFS{
		"bad.json": {Data: []byte(`{"id":"x","intent":"x","kind":"frobnicate","handler":"h","rbac":{"requires_permission":"report.view"}}`)},
	}

	_, err := Load(fsys, validPermsFixture())
	if err == nil {
		t.Fatal("expected error for bad kind")
	}
}

func TestLoadRejectsMalformedJSON(t *testing.T) {
	fsys := fstest.MapFS{"bad.json": {Data: []byte(`{not json`)}}

	if _, err := Load(fsys, validPermsFixture()); err == nil {
		t.Fatal("expected parse error")
	}
}
