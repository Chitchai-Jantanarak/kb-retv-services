package graph

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/memgraph"
)

const (
	LabelReport    = "Report"
	LabelKbArticle = "KbArticle"
	LabelSymptom   = "Symptom"

	EdgeSolves   = "SOLVES"
	EdgeProduced = "PRODUCED"
)

type KBGraph struct {
	store ports.GraphStore
}

func NewKBGraph(store ports.GraphStore) *KBGraph {
	return &KBGraph{store: store}
}

func (g *KBGraph) UpsertReport(ctx context.Context, companyID int64, reportID int64, props map[string]any) error {
	return g.upsertEntity(ctx, companyID, LabelReport, "report", strconv.FormatInt(reportID, 10), props)
}

func (g *KBGraph) UpsertArticle(ctx context.Context, companyID int64, articleID int64, props map[string]any) error {
	return g.upsertEntity(ctx, companyID, LabelKbArticle, "article", strconv.FormatInt(articleID, 10), props)
}

func (g *KBGraph) UpsertSymptom(ctx context.Context, companyID int64, symptomID int64, props map[string]any) error {
	return g.upsertEntity(ctx, companyID, LabelSymptom, "symptom", strconv.FormatInt(symptomID, 10), props)
}

func (g *KBGraph) LinkArticleSolvesSymptom(ctx context.Context, companyID int64, articleID, symptomID int64, props map[string]any) error {
	from := memgraph.ExternalID(companyID, "article", strconv.FormatInt(articleID, 10))
	to := memgraph.ExternalID(companyID, "symptom", strconv.FormatInt(symptomID, 10))
	return g.upsertEdge(ctx, companyID, from, to, EdgeSolves, props)
}

func (g *KBGraph) LinkReportProducedArticle(ctx context.Context, companyID int64, reportID, articleID int64) error {
	from := memgraph.ExternalID(companyID, "report", strconv.FormatInt(reportID, 10))
	to := memgraph.ExternalID(companyID, "article", strconv.FormatInt(articleID, 10))
	return g.upsertEdge(ctx, companyID, from, to, EdgeProduced, nil)
}

// ArticlesForSymptom returns articles that SOLVE the given symptom,
// scoped to the company. Limit is enforced by Cypher.
func (g *KBGraph) ArticlesForSymptom(ctx context.Context, companyID int64, coverage []int64, symptomID int64, limit int) ([]map[string]any, error) {
	if companyID <= 0 {
		return nil, errors.New("kb_graph: ArticlesForSymptom: company_id must be positive")
	}
	if limit <= 0 {
		limit = 16
	}
	scope := coverageOrSelf(coverage, companyID)
	stmt := fmt.Sprintf(`
		MATCH (a:%s)-[r:%s { company_id: $company_id }]->(s:%s { company_id: $company_id, id: $symptom_id })
		WHERE a.company_id IN $coverage
		RETURN a.id AS article_id, a.title AS title, r.score AS score, a.source_report_id AS source_report_id
		ORDER BY r.score DESC
		LIMIT %d
	`, LabelKbArticle, EdgeSolves, LabelSymptom, limit)
	return g.store.Query(ctx, ports.CypherQuery{
		Statement: stmt,
		Params: map[string]any{
			"company_id": companyID,
			"coverage":   scope,
			"symptom_id": memgraph.ExternalID(companyID, "symptom", strconv.FormatInt(symptomID, 10)),
		},
	})
}

func (g *KBGraph) upsertEntity(ctx context.Context, companyID int64, label, kind, key string, props map[string]any) error {
	if companyID <= 0 {
		return fmt.Errorf("kb_graph: %s: company_id must be positive", label)
	}
	if key == "" {
		return fmt.Errorf("kb_graph: %s: id must not be empty", label)
	}
	return g.store.UpsertNode(ctx, ports.Node{
		CompanyID: companyID,
		ID:        memgraph.ExternalID(companyID, kind, key),
		Labels:    []string{label},
		Props:     props,
	})
}

func (g *KBGraph) upsertEdge(ctx context.Context, companyID int64, from, to, edgeType string, props map[string]any) error {
	if companyID <= 0 {
		return fmt.Errorf("kb_graph: edge %s: company_id must be positive", edgeType)
	}
	return g.store.UpsertEdge(ctx, ports.Edge{
		CompanyID: companyID,
		From:      from,
		To:        to,
		Type:      edgeType,
		Props:     props,
	})
}
