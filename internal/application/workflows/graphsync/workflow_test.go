package graphsync

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/repositories/graph"
)

type stubGraphStore struct {
	nodes []ports.Node
	edges []ports.Edge
}

func (s *stubGraphStore) UpsertNode(_ context.Context, n ports.Node) error {
	s.nodes = append(s.nodes, n)
	return nil
}

func (s *stubGraphStore) UpsertEdge(_ context.Context, e ports.Edge) error {
	s.edges = append(s.edges, e)
	return nil
}

func (s *stubGraphStore) Query(_ context.Context, _ ports.CypherQuery) ([]map[string]any, error) {
	return nil, nil
}

type stubArticleSource struct {
	articles   []ArticleRecord
	skipFilter bool
	err        error
	calls      int
}

func (s *stubArticleSource) ArticlesSince(_ context.Context, companyID int64, _ time.Time, _ int) ([]ArticleRecord, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if s.skipFilter {
		return append([]ArticleRecord(nil), s.articles...), nil
	}
	out := make([]ArticleRecord, 0)
	for _, a := range s.articles {
		if a.CompanyID == companyID {
			out = append(out, a)
		}
	}
	return out, nil
}

func TestWorkflowFailureCases(t *testing.T) {
	cases := []struct {
		name      string
		companyID int64
		articles  []ArticleRecord
		sourceErr error
		wantSub   string
	}{
		{
			name:      "rejects_zero_company",
			companyID: 0,
			wantSub:   "company_id must be positive",
		},
		{
			name:      "source_failure_bubbles",
			companyID: 7,
			sourceErr: errors.New("db down"),
			wantSub:   "list articles",
		},
		{
			name:      "cross_tenant_record_aborts",
			companyID: 7,
			articles:  []ArticleRecord{{ID: 1, CompanyID: 8, SourceReportID: 10, Title: "X"}},
			wantSub:   "belongs to company",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := &stubArticleSource{
				articles:   tc.articles,
				err:        tc.sourceErr,
				skipFilter: tc.name == "cross_tenant_record_aborts",
			}
			store := &stubGraphStore{}
			wf, err := New(Config{Source: src, Graph: graph.NewKBGraph(store)})
			if err != nil {
				t.Fatalf("New() err = %v", err)
			}

			_, err = wf.Run(context.Background(), Options{CompanyID: tc.companyID})
			if err == nil {
				t.Fatalf("Run() = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestWorkflowEmitsReportArticleAndEdge(t *testing.T) {
	src := &stubArticleSource{articles: []ArticleRecord{
		{ID: 11, CompanyID: 7, SourceReportID: 101, Title: "First"},
		{ID: 12, CompanyID: 7, SourceReportID: 0, Title: "Orphan"},
		{ID: 13, CompanyID: 8, SourceReportID: 999, Title: "Other"},
	}}
	store := &stubGraphStore{}
	wf, err := New(Config{Source: src, Graph: graph.NewKBGraph(store)})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	res, err := wf.Run(context.Background(), Options{CompanyID: 7})
	if err != nil {
		t.Fatalf("Run() err = %v", err)
	}
	if res.Articles != 2 {
		t.Fatalf("res.Articles = %d, want 2", res.Articles)
	}
	if res.Edges != 1 {
		t.Fatalf("res.Edges = %d, want 1", res.Edges)
	}
	if res.Skipped != 1 {
		t.Fatalf("res.Skipped = %d, want 1 (orphan article)", res.Skipped)
	}

	for _, n := range store.nodes {
		if n.CompanyID != 7 {
			t.Fatalf("node leaked: %+v", n)
		}
	}
	for _, e := range store.edges {
		if e.CompanyID != 7 || e.Type != graph.EdgeProduced {
			t.Fatalf("unexpected edge: %+v", e)
		}
	}
}

func TestWorkflowLinksArticleToClassifiedSymptom(t *testing.T) {
	src := &stubArticleSource{articles: []ArticleRecord{
		{ID: 11, CompanyID: 7, SourceReportID: 101, Title: "First", SymptomID: 55, SymptomName: "wifi drops"},
	}}
	store := &stubGraphStore{}
	wf, err := New(Config{Source: src, Graph: graph.NewKBGraph(store)})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	res, err := wf.Run(context.Background(), Options{CompanyID: 7})
	if err != nil {
		t.Fatalf("Run() err = %v", err)
	}
	if res.Edges != 2 {
		t.Fatalf("res.Edges = %d, want 2 (produced + solves)", res.Edges)
	}

	var sawSymptomNode, sawSolvesEdge bool
	for _, n := range store.nodes {
		if len(n.Labels) == 1 && n.Labels[0] == graph.LabelSymptom {
			sawSymptomNode = true
			if n.CompanyID != 7 || n.ID != "7:symptom:55" {
				t.Fatalf("unexpected symptom node: %+v", n)
			}
		}
	}
	for _, e := range store.edges {
		if e.Type == graph.EdgeSolves {
			sawSolvesEdge = true
			if e.CompanyID != 7 || e.From != "7:article:11" || e.To != "7:symptom:55" {
				t.Fatalf("unexpected solves edge: %+v", e)
			}
		}
	}
	if !sawSymptomNode {
		t.Fatal("expected a Symptom node to be upserted")
	}
	if !sawSolvesEdge {
		t.Fatal("expected a SOLVES edge to be created")
	}
}

func TestWorkflowNoSymptomSkipsSolvesEdge(t *testing.T) {
	src := &stubArticleSource{articles: []ArticleRecord{
		{ID: 11, CompanyID: 7, SourceReportID: 101, Title: "First"},
	}}
	store := &stubGraphStore{}
	wf, err := New(Config{Source: src, Graph: graph.NewKBGraph(store)})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	res, err := wf.Run(context.Background(), Options{CompanyID: 7})
	if err != nil {
		t.Fatalf("Run() err = %v", err)
	}
	if res.Edges != 1 {
		t.Fatalf("res.Edges = %d, want 1 (produced only)", res.Edges)
	}
	for _, e := range store.edges {
		if e.Type == graph.EdgeSolves {
			t.Fatalf("unexpected solves edge emitted: %+v", e)
		}
	}
	for _, n := range store.nodes {
		if len(n.Labels) == 1 && n.Labels[0] == graph.LabelSymptom {
			t.Fatalf("unexpected symptom node emitted: %+v", n)
		}
	}
}

func TestWorkflowContextCancellationPropagates(t *testing.T) {
	src := &stubArticleSource{articles: []ArticleRecord{
		{ID: 11, CompanyID: 7, SourceReportID: 101, Title: "First"},
	}}
	store := &stubGraphStore{}
	wf, err := New(Config{Source: src, Graph: graph.NewKBGraph(store)})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = wf.Run(ctx, Options{CompanyID: 7})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if src.calls != 0 {
		t.Fatalf("source.calls = %d, want 0", src.calls)
	}
}
