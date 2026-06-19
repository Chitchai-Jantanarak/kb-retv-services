package graph

import (
	"context"
	"strings"
	"testing"

	"github.com/my/app/internal/domain/ports"
)

type fakeGraphStore struct {
	nodes    []ports.Node
	edges    []ports.Edge
	queries  []ports.CypherQuery
	queryErr error
}

func (f *fakeGraphStore) UpsertNode(_ context.Context, n ports.Node) error {
	f.nodes = append(f.nodes, n)
	return nil
}

func (f *fakeGraphStore) UpsertEdge(_ context.Context, e ports.Edge) error {
	f.edges = append(f.edges, e)
	return nil
}

func (f *fakeGraphStore) Query(_ context.Context, q ports.CypherQuery) ([]map[string]any, error) {
	f.queries = append(f.queries, q)
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return nil, nil
}

func TestKBGraphUpsertRejectsZeroCompany(t *testing.T) {
	store := &fakeGraphStore{}
	g := NewKBGraph(store)

	cases := []struct {
		name string
		fn   func() error
		want string
	}{
		{
			name: "report",
			fn:   func() error { return g.UpsertReport(context.Background(), 0, 1, nil) },
			want: "company_id must be positive",
		},
		{
			name: "article",
			fn:   func() error { return g.UpsertArticle(context.Background(), -1, 1, nil) },
			want: "company_id must be positive",
		},
		{
			name: "symptom",
			fn:   func() error { return g.UpsertSymptom(context.Background(), 0, 1, nil) },
			want: "company_id must be positive",
		},
		{
			name: "edge_solves",
			fn: func() error {
				return g.LinkArticleSolvesSymptom(context.Background(), 0, 1, 2, nil)
			},
			want: "company_id must be positive",
		},
		{
			name: "articles_for_symptom_traversal",
			fn: func() error {
				_, err := g.ArticlesForSymptom(context.Background(), 0, nil, 1, 10)
				return err
			},
			want: "company_id must be positive",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			if err == nil {
				t.Fatalf("got nil error, want %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
	if len(store.nodes) != 0 || len(store.edges) != 0 || len(store.queries) != 0 {
		t.Fatalf("store touched on rejected calls: nodes=%d edges=%d queries=%d", len(store.nodes), len(store.edges), len(store.queries))
	}
}

func TestKBGraphUpsertEntityCarriesCompanyID(t *testing.T) {
	store := &fakeGraphStore{}
	g := NewKBGraph(store)

	if err := g.UpsertArticle(context.Background(), 7, 42, map[string]any{"title": "Boot"}); err != nil {
		t.Fatalf("UpsertArticle err = %v", err)
	}
	if len(store.nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(store.nodes))
	}
	n := store.nodes[0]
	if n.CompanyID != 7 {
		t.Fatalf("node.CompanyID = %d, want 7", n.CompanyID)
	}
	if n.ID != "7:article:42" {
		t.Fatalf("node.ID = %q, want 7:article:42", n.ID)
	}
	if len(n.Labels) != 1 || n.Labels[0] != LabelKbArticle {
		t.Fatalf("labels = %v, want [%s]", n.Labels, LabelKbArticle)
	}
}

func TestKBGraphLinkSolvesCarriesCompanyAndType(t *testing.T) {
	store := &fakeGraphStore{}
	g := NewKBGraph(store)

	if err := g.LinkArticleSolvesSymptom(context.Background(), 9, 100, 200, map[string]any{"score": 0.91}); err != nil {
		t.Fatalf("LinkArticleSolvesSymptom err = %v", err)
	}
	if len(store.edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(store.edges))
	}
	e := store.edges[0]
	if e.CompanyID != 9 {
		t.Fatalf("edge.CompanyID = %d, want 9", e.CompanyID)
	}
	if e.Type != EdgeSolves {
		t.Fatalf("edge.Type = %q, want %q", e.Type, EdgeSolves)
	}
	if e.From != "9:article:100" || e.To != "9:symptom:200" {
		t.Fatalf("from/to = %q/%q, want 9:article:100 / 9:symptom:200", e.From, e.To)
	}
}

func TestArticlesForSymptomScopesCompanyInQuery(t *testing.T) {
	store := &fakeGraphStore{}
	g := NewKBGraph(store)

	if _, err := g.ArticlesForSymptom(context.Background(), 3, []int64{3, 8}, 50, 5); err != nil {
		t.Fatalf("ArticlesForSymptom err = %v", err)
	}
	if len(store.queries) != 1 {
		t.Fatalf("queries = %d, want 1", len(store.queries))
	}
	q := store.queries[0]
	if !strings.Contains(q.Statement, "company_id: $company_id") {
		t.Fatalf("statement missing company_id scope:\n%s", q.Statement)
	}
	if q.Params["company_id"] != int64(3) {
		t.Fatalf("params.company_id = %v, want 3", q.Params["company_id"])
	}
	if q.Params["symptom_id"] != "3:symptom:50" {
		t.Fatalf("params.symptom_id = %v, want 3:symptom:50", q.Params["symptom_id"])
	}
	if !strings.Contains(q.Statement, "a.company_id IN $coverage") {
		t.Fatalf("statement missing coverage filter:\n%s", q.Statement)
	}
	cov, ok := q.Params["coverage"].([]int64)
	if !ok || len(cov) != 2 || cov[0] != 3 || cov[1] != 8 {
		t.Fatalf("params.coverage = %v, want [3 8]", q.Params["coverage"])
	}
}
