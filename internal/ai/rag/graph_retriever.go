package rag

import (
	"context"
	"fmt"
)

type GraphArticleSource interface {
	ArticlesForSymptom(ctx context.Context, companyID int64, coverage []int64, symptomID int64, limit int) ([]map[string]any, error)
}

type GraphAnchorRetriever struct {
	source GraphArticleSource
}

func NewGraphAnchorRetriever(source GraphArticleSource) *GraphAnchorRetriever {
	return &GraphAnchorRetriever{source: source}
}

func (r *GraphAnchorRetriever) RetrieveGraph(ctx context.Context, query Query, plan QueryPlan) ([]Candidate, error) {
	if r.source == nil || query.CompanyID <= 0 {
		return nil, nil
	}
	limit := vectorLimit(query)
	coverage := queryCoverage(ctx, query, query.CompanyID)
	var sets [][]Candidate
	for _, anchor := range plan.GraphAnchors {
		if anchor.Kind != GraphAnchorSymptom || anchor.ID <= 0 {
			continue
		}
		rows, err := r.source.ArticlesForSymptom(ctx, query.CompanyID, coverage, anchor.ID, limit)
		if err != nil {
			return nil, err
		}
		sets = append(sets, graphRowsToCandidates(rows))
	}
	return rrfMerge(sets), nil
}

func graphRowsToCandidates(rows []map[string]any) []Candidate {
	out := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		id := graphString(row["article_id"])
		if id == "" {
			continue
		}
		title := graphString(row["title"])
		content := graphString(row["content"])
		if content == "" {
			content = title
		}
		out = append(out, Candidate{
			ID:      id,
			Title:   title,
			Content: content,
			Source:  "graph",
			Score:   graphFloat(row["score"]),
		})
	}
	return out
}

func graphString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func graphFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 1
	}
}

var _ GraphRetriever = (*GraphAnchorRetriever)(nil)
