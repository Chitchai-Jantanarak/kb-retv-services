package tools

import (
	"context"
	"testing"
)

type recordingNames struct {
	lookups []string
	byName  map[string][]string
}

func (r *recordingNames) Names(_ context.Context, lookup string) ([]string, error) {
	r.lookups = append(r.lookups, lookup)
	return r.byName[lookup], nil
}

type fixedEmbedder struct{ vec []float32 }

func (f fixedEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = f.vec
	}
	return out, nil
}

func rbacFixture(t *testing.T, names NameSource) *Selector {
	t.Helper()
	catalog := []Tool{
		{
			ID:      "staff_tool",
			Anchors: []string{"who is on the team"},
			RBAC:    RBAC{RequiresPermission: "team.view"},
			Params:  []Param{{Name: "employee", Required: true, Lookup: "employees.name"}},
		},
		{
			ID:      "case_tool",
			Anchors: []string{"find cases"},
			RBAC:    RBAC{RequiresPermission: "report.view"},
			Params:  []Param{{Name: "product", Required: true, Lookup: "nodes.title"}},
		},
	}
	sel, err := NewSelector(context.Background(), fixedEmbedder{vec: []float32{1, 0}}, catalog,
		WithNameSource(names))
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}
	return sel
}

func TestSelectDoesNotReadLookupsForUnpermittedTools(t *testing.T) {
	names := &recordingNames{byName: map[string][]string{
		"employees.name": {"Somchai Prasert"},
		"nodes.title":    {"Bella Bot"},
	}}
	sel := rbacFixture(t, names)

	if _, err := sel.Select(context.Background(), "somchai prasert bella bot", []string{"report.view"}); err != nil {
		t.Fatalf("Select: %v", err)
	}

	for _, lookup := range names.lookups {
		if lookup == "employees.name" {
			t.Fatalf("lookups = %v; employees.name was read for a tool the principal cannot call", names.lookups)
		}
	}
	if len(names.lookups) == 0 {
		t.Fatal("expected the permitted tool's lookup to still run")
	}
}

func TestSelectReadsNoLookupWhenEveryToolIsFiltered(t *testing.T) {
	names := &recordingNames{byName: map[string][]string{
		"employees.name": {"Somchai Prasert"},
		"nodes.title":    {"Bella Bot"},
	}}
	sel := rbacFixture(t, names)

	got, err := sel.Select(context.Background(), "somchai prasert bella bot", []string{"unrelated.perm"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(names.lookups) != 0 {
		t.Fatalf("lookups = %v; a fully filtered principal must not reach the gazetteer", names.lookups)
	}
	if got.Matched {
		t.Fatalf("got %+v, want no match when every tool is permission filtered", got)
	}
	if !got.PermFiltered {
		t.Fatal("PermFiltered must report that RBAC, not intent, caused the miss")
	}
}
