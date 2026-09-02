package tools

import (
	"os"
	"testing"
)

func leafVocab() map[string]bool {
	keys := []string{
		"console.access",
		"company.create", "company.update", "company.archive", "company.reparent", "company.admin",
		"report.view", "report.create", "report.update", "report.delete", "report.assign",
		"employee.view", "team.view",
		"customer.view", "site.view", "node.view",
		"kb.search", "portal.view",
	}
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

func TestConfigToolsAreValid(t *testing.T) {
	fsys := os.DirFS("../../../config/tools")

	got, err := Load(fsys, leafVocab())
	if err != nil {
		t.Fatalf("config tools invalid: %v", err)
	}
	if len(got) != 13 {
		t.Fatalf("want 13 tools, got %d", len(got))
	}
}
