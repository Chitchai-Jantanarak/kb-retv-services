package search

import (
	"context"
	"math"
	"testing"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

type fakeEmbedder struct{}

func (fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{0.1, 0.2, 0.3}
	}
	return out, nil
}

func (fakeEmbedder) Dim() int { return 3 }

type fakeVector struct {
	hits []ports.VectorHit
}

func (f fakeVector) EnsureCollection(context.Context, string, int) error { return nil }
func (f fakeVector) Upsert(context.Context, string, []ports.VectorPoint) error {
	return nil
}
func (f fakeVector) Delete(context.Context, string, []string) error { return nil }
func (f fakeVector) Search(context.Context, ports.VectorSearch) ([]ports.VectorHit, error) {
	return f.hits, nil
}

type fakeCases struct {
	rows     []CaseRecord
	gotIDs   []int64
	gotCover []int64
}

func (f *fakeCases) CasesByIDs(_ context.Context, coverage []int64, ids []int64) ([]CaseRecord, error) {
	f.gotIDs = ids
	f.gotCover = coverage
	return f.rows, nil
}

type fakeGraph struct {
	bySymptom map[int64][]map[string]any
	calls     int
}

func (f *fakeGraph) ArticlesForSymptom(_ context.Context, _ int64, _ []int64, symptomID int64, _ int) ([]map[string]any, error) {
	f.calls++
	return f.bySymptom[symptomID], nil
}

func testCtx() context.Context {
	return ctxkey.WithCompanyID(context.Background(), 7)
}

func TestRunMapsVectorHitsToReportsAndKeepsBestScore(t *testing.T) {
	vector := fakeVector{hits: []ports.VectorHit{
		{ID: "c1", Score: 0.4, Metadata: map[string]any{"source_report_id": float64(11)}},
		{ID: "c2", Score: 0.9, Metadata: map[string]any{"source_report_id": float64(11)}},
		{ID: "c3", Score: 0.7, Metadata: map[string]any{"source_report_id": "12"}},
		{ID: "c4", Score: 0.6, Metadata: map[string]any{}},
	}}
	cases := &fakeCases{rows: []CaseRecord{
		{ID: 11, Code: "REP-11", Title: "robot stuck", Status: "waiting"},
		{ID: 12, Code: "REP-12", Title: "battery flat", Status: "process"},
	}}

	wf, err := New(Config{Embedder: fakeEmbedder{}, Vector: vector, Cases: cases})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(testCtx(), dto.SearchRequest{Query: "robot", Types: []string{dto.SearchTypeReport}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(cases.gotIDs) != 2 {
		t.Fatalf("gotIDs = %v, want 2 deduped report ids (hit without source_report_id must be dropped)", cases.gotIDs)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("results = %+v, want 2", resp.Results)
	}
	if resp.Results[0].ReportID != 11 || math.Abs(resp.Results[0].Score-0.9) > 1e-6 {
		t.Fatalf("top result = %+v, want report 11 at best-of-chunks score 0.9", resp.Results[0])
	}
	if resp.Results[0].Source != "vector" || resp.Results[0].Type != dto.SearchTypeReport {
		t.Fatalf("result shape = %+v, want vector-sourced report", resp.Results[0])
	}
	if resp.GraphUsed {
		t.Fatalf("GraphUsed = true with no graph source configured")
	}
}

func TestRunSkipsGraphWhenNotConfigured(t *testing.T) {
	vector := fakeVector{hits: []ports.VectorHit{
		{ID: "c1", Score: 0.5, Metadata: map[string]any{"source_report_id": int64(11)}},
	}}
	cases := &fakeCases{rows: []CaseRecord{{ID: 11, SymptomID: 99}}}

	wf, err := New(Config{Embedder: fakeEmbedder{}, Vector: vector, Cases: cases})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(testCtx(), dto.SearchRequest{Query: "robot"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.GraphUsed {
		t.Fatalf("GraphUsed = true, want false when MEMGRAPH_ENABLED left the graph half nil")
	}
	for _, r := range resp.Results {
		if r.Type == dto.SearchTypeKB {
			t.Fatalf("results = %+v, want no kb rows without a graph source", resp.Results)
		}
	}
}

func TestRunExpandsGraphOncePerSymptomAndDedupesArticles(t *testing.T) {
	vector := fakeVector{hits: []ports.VectorHit{
		{ID: "c1", Score: 0.5, Metadata: map[string]any{"source_report_id": int64(11)}},
		{ID: "c2", Score: 0.4, Metadata: map[string]any{"source_report_id": int64(12)}},
	}}
	cases := &fakeCases{rows: []CaseRecord{
		{ID: 11, SymptomID: 99},
		{ID: 12, SymptomID: 99},
	}}
	graph := &fakeGraph{bySymptom: map[int64][]map[string]any{
		99: {
			{"article_id": "5", "title": "reset the docking sensor", "score": 0.8},
			{"article_id": "5", "title": "duplicate", "score": 0.8},
		},
	}}

	wf, err := New(Config{Embedder: fakeEmbedder{}, Vector: vector, Cases: cases, Graph: graph})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(testCtx(), dto.SearchRequest{Query: "robot"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if graph.calls != 1 {
		t.Fatalf("graph calls = %d, want 1 — both reports share symptom 99", graph.calls)
	}
	if !resp.GraphUsed {
		t.Fatalf("GraphUsed = false, want true")
	}
	kb := 0
	for _, r := range resp.Results {
		if r.Type == dto.SearchTypeKB {
			kb++
			if r.ArticleID != "5" || r.Source != "graph" {
				t.Fatalf("kb result = %+v, want article 5 sourced from graph", r)
			}
		}
	}
	if kb != 1 {
		t.Fatalf("kb results = %d, want 1 after dedupe", kb)
	}
}

func TestRunRejectsMissingCompanyScope(t *testing.T) {
	wf, err := New(Config{Embedder: fakeEmbedder{}, Vector: fakeVector{}, Cases: &fakeCases{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := wf.Run(context.Background(), dto.SearchRequest{Query: "robot"}); err == nil {
		t.Fatal("Run without company scope returned nil error, want unauthorized")
	}
}

func TestRunRejectsEmptyQuery(t *testing.T) {
	wf, err := New(Config{Embedder: fakeEmbedder{}, Vector: fakeVector{}, Cases: &fakeCases{}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := wf.Run(testCtx(), dto.SearchRequest{Query: "   "}); err == nil {
		t.Fatal("Run with blank query returned nil error, want invalid input")
	}
}
