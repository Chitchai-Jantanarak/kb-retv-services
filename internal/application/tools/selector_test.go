package tools

import (
	"context"
	"testing"
)

type fakeEmbedder struct {
	table map[string][]float32
}

func (f fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		if v, ok := f.table[t]; ok {
			out[i] = v
			continue
		}
		out[i] = []float32{0, 0}
	}
	return out, nil
}

func twoTools() []Tool {
	return []Tool{
		{ID: "f1", Anchors: []string{"cases"}, Thresholds: Thresholds{Accept: 0.55}, RBAC: RBAC{RequiresPermission: "report.view"}},
		{ID: "f4", Anchors: []string{"workload"}, Thresholds: Thresholds{Accept: 0.55}, RBAC: RBAC{RequiresPermission: "team.view"}},
	}
}

func emb() fakeEmbedder {
	return fakeEmbedder{table: map[string][]float32{
		"cases":         {1, 0},
		"workload":      {0, 1},
		"show me cases": {1, 0},
		"team workload": {0, 1},
	}}
}

func TestSelectPicksAllowedTool(t *testing.T) {
	s, err := NewSelector(context.Background(), emb(), twoTools())
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}

	got, err := s.Select(context.Background(), "show me cases", []string{"report.view", "team.view"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got.ToolID != "f1" || !got.Matched {
		t.Fatalf("want f1 matched, got %+v", got)
	}
}

func TestSelectFiltersByPermission(t *testing.T) {
	s, _ := NewSelector(context.Background(), emb(), twoTools())

	got, _ := s.Select(context.Background(), "show me cases", []string{"team.view"})
	if got.ToolID == "f1" {
		t.Fatalf("f1 should be filtered out (no report.view), got %+v", got)
	}
	if got.Matched {
		t.Fatalf("no allowed tool should match the cases query, got %+v", got)
	}
}

func TestSelectStarGrantsAll(t *testing.T) {
	s, _ := NewSelector(context.Background(), emb(), twoTools())

	got, _ := s.Select(context.Background(), "team workload", []string{"*"})
	if got.ToolID != "f4" || !got.Matched {
		t.Fatalf("want f4 matched under *, got %+v", got)
	}
}

func TestSelectNoMatchBelowThreshold(t *testing.T) {
	s, _ := NewSelector(context.Background(), emb(), twoTools())

	got, _ := s.Select(context.Background(), "totally unrelated", []string{"*"})
	if got.Matched {
		t.Fatalf("unrelated query should not match, got %+v", got)
	}
}

func thresholdRaceTools() []Tool {
	return []Tool{
		{ID: "high-score-fails", Anchors: []string{"a"}, Thresholds: Thresholds{Accept: 0.62}, RBAC: RBAC{RequiresPermission: "report.view"}},
		{ID: "low-score-clears", Anchors: []string{"b"}, Thresholds: Thresholds{Accept: 0.50}, RBAC: RBAC{RequiresPermission: "report.view"}},
	}
}

func thresholdRaceEmb() fakeEmbedder {
	return fakeEmbedder{table: map[string][]float32{
		"a":     {1, 0},
		"b":     {0.9981, -0.061},
		"query": {3, 4},
	}}
}

func TestSelectPrefersThresholdClearingToolOverHigherScore(t *testing.T) {
	s, err := NewSelector(context.Background(), thresholdRaceEmb(), thresholdRaceTools())
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}

	got, err := s.Select(context.Background(), "query", []string{"report.view"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got.ToolID != "low-score-clears" || !got.Matched {
		t.Fatalf("want low-score-clears matched (its own threshold cleared), got %+v", got)
	}
}

func nearMissTools() []Tool {
	return []Tool{
		{ID: "closer-but-still-far", Anchors: []string{"a"}, Thresholds: Thresholds{Accept: 0.90}, RBAC: RBAC{RequiresPermission: "report.view"}},
		{ID: "farther-still", Anchors: []string{"b"}, Thresholds: Thresholds{Accept: 0.90}, RBAC: RBAC{RequiresPermission: "report.view"}},
	}
}

func TestSelectReturnsNearMissWhenNothingClears(t *testing.T) {
	s, err := NewSelector(context.Background(), thresholdRaceEmb(), nearMissTools())
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}

	got, err := s.Select(context.Background(), "query", []string{"report.view"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got.Matched {
		t.Fatalf("nothing should clear its threshold, got %+v", got)
	}
	if got.ToolID != "closer-but-still-far" {
		t.Fatalf("want near-miss ToolID to be the highest-scoring tool, got %+v", got)
	}
	if got.Score <= 0 {
		t.Fatalf("want near-miss Score to be surfaced (nonzero), got %+v", got)
	}
}

func TestSelectMarksPermFiltered(t *testing.T) {
	s, err := NewSelector(context.Background(), emb(), twoTools())
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}

	got, err := s.Select(context.Background(), "show me cases", nil)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got.Matched || !got.PermFiltered {
		t.Fatalf("want unmatched + PermFiltered with no grants, got %+v", got)
	}

	got, err = s.Select(context.Background(), "show me cases", []string{"report.view", "team.view"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got.PermFiltered {
		t.Fatalf("PermFiltered should be false when tools passed perms, got %+v", got)
	}
}

func caseTools() []Tool {
	codeParam := Param{Name: "code", Required: true, Pattern: `(?i)\bREP-?\d+\b`}
	return []Tool{
		{ID: "f1_find", Anchors: []string{"latest cases"}, Thresholds: Thresholds{Accept: 0.55}, RBAC: RBAC{RequiresPermission: "report.view"},
			Params: []Param{{Name: "status", Map: map[string]string{"waiting": "waiting", "done": "done"}}}},
		{ID: "f2_track", Anchors: []string{"case status"}, Thresholds: Thresholds{Accept: 0.55}, RBAC: RBAC{RequiresPermission: "report.view"},
			Params: []Param{codeParam}},
	}
}

func caseEmb() fakeEmbedder {
	return fakeEmbedder{table: map[string][]float32{
		"latest cases":                 {1, 0},
		"case status":                  {0.8, 0.6},
		"give me the latest cases":     {0.99, 0.14},
		"status of REP-606":            {0.85, 0.53},
		"what is the status of a case": {0.85, 0.53},
	}}
}

func TestSelectExcludesUnsatisfiableTool(t *testing.T) {
	s, err := NewSelector(context.Background(), caseEmb(), caseTools())
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}

	got, err := s.Select(context.Background(), "what is the status of a case", []string{"report.view"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got.ToolID != "f1_find" || !got.Matched {
		t.Fatalf("want f1_find (f2 unsatisfiable without code), got %+v", got)
	}
}

func TestSelectPrefersMoreSpecificSatisfiableTool(t *testing.T) {
	s, err := NewSelector(context.Background(), caseEmb(), caseTools())
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}

	got, err := s.Select(context.Background(), "status of REP-606", []string{"report.view"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got.ToolID != "f2_track" || !got.Matched {
		t.Fatalf("want f2_track to beat f1 on specificity when a code is present, got %+v", got)
	}
	if got.Params["code"] != "REP-606" {
		t.Fatalf("params = %v, want code REP-606", got.Params)
	}
}

func TestSelectSurfacesOptionalParams(t *testing.T) {
	s, err := NewSelector(context.Background(), caseEmb(), caseTools())
	if err != nil {
		t.Fatalf("NewSelector: %v", err)
	}

	got, err := s.Select(context.Background(), "give me the latest cases", []string{"report.view"})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got.ToolID != "f1_find" {
		t.Fatalf("want f1_find, got %+v", got)
	}
	if len(got.Params) != 0 {
		t.Fatalf("params = %v, want empty when no keyword present", got.Params)
	}
}
