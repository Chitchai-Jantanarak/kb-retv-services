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
		"cases":          {1, 0},
		"workload":       {0, 1},
		"show me cases":  {1, 0},
		"team workload":  {0, 1},
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
