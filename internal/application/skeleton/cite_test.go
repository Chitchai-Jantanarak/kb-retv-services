package skeleton

import (
	"testing"

	"github.com/my/app/internal/application/tools"
)

func TestResponseCarriesCiteKeyAndValues(t *testing.T) {
	tool := tools.Tool{
		ID:   "f1_find_cases",
		Kind: "read",
		Compose: tools.Compose{
			Mode:     "summary",
			Headline: "{n} cases",
			Cite:     "code",
			Columns: []tools.ToolColumn{
				{Key: "code", Type: "case_ref", Primary: true},
				{Key: "title", Type: "text"},
			},
		},
	}
	rows := []Row{
		{"code": "REP-4106", "title": "first"},
		{"code": "REP-4105", "title": "second"},
	}

	got := compose(tool, rows, nil)

	if got.Cite != "code" {
		t.Fatalf("Cite = %q, want %q", got.Cite, "code")
	}
	want := []string{"REP-4106", "REP-4105"}
	if len(got.CiteValues) != len(want) {
		t.Fatalf("CiteValues = %v, want %v", got.CiteValues, want)
	}
	for i, v := range want {
		if got.CiteValues[i] != v {
			t.Fatalf("CiteValues[%d] = %q, want %q", i, got.CiteValues[i], v)
		}
	}
}

func TestResponseCiteValuesEmptyWhenToolDeclaresNoCite(t *testing.T) {
	tool := tools.Tool{ID: "f4_workload", Kind: "read", Compose: tools.Compose{Mode: "template"}}
	got := compose(tool, []Row{{"name": "somchai"}}, nil)

	if got.Cite != "" || len(got.CiteValues) != 0 {
		t.Fatalf("Cite=%q CiteValues=%v, want both empty", got.Cite, got.CiteValues)
	}
}
