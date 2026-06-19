package rag

import (
	"context"
	"testing"
)

func TestGraphAnchorRetrieverUsesSymptomAnchors(t *testing.T) {
	source := &fakeGraphArticleSource{
		rows: []map[string]any{
			{"article_id": "a1", "title": "Reset tray", "score": 0.8},
		},
	}
	retriever := NewGraphAnchorRetriever(source)

	candidates, err := retriever.RetrieveGraph(context.Background(), Query{
		CompanyID: 7,
		Text:      "tray jam",
		Limit:     3,
	}, QueryPlan{
		GraphAnchors: []GraphAnchor{{Kind: GraphAnchorSymptom, ID: 44}},
	})
	if err != nil {
		t.Fatalf("RetrieveGraph: %v", err)
	}
	if source.companyID != 7 || source.symptomID != 44 || source.limit != 3 {
		t.Fatalf("source call company=%d symptom=%d limit=%d", source.companyID, source.symptomID, source.limit)
	}
	if len(source.coverage) != 1 || source.coverage[0] != 7 {
		t.Fatalf("source coverage = %v, want [7]", source.coverage)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %+v", candidates)
	}
	if candidates[0].ID != "a1" || candidates[0].Title != "Reset tray" || candidates[0].Source != "graph" {
		t.Fatalf("candidate = %+v", candidates[0])
	}
}

type fakeGraphArticleSource struct {
	companyID int64
	coverage  []int64
	symptomID int64
	limit     int
	rows      []map[string]any
}

func (s *fakeGraphArticleSource) ArticlesForSymptom(_ context.Context, companyID int64, coverage []int64, symptomID int64, limit int) ([]map[string]any, error) {
	s.companyID = companyID
	s.coverage = coverage
	s.symptomID = symptomID
	s.limit = limit
	return s.rows, nil
}
