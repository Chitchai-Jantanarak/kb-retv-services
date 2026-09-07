package handlers

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
	"github.com/my/app/internal/transport/http/response"
)

type KnowledgeGraphQuerier interface {
	Query(ctx context.Context, q ports.CypherQuery) ([]map[string]any, error)
}

type KnowledgeVectorInfo interface {
	CollectionInfo(ctx context.Context, name string) (points int, dim int, exists bool, err error)
}

type knowledgeCollectionNamer interface {
	CollectionForCompany(companyID int64) (string, error)
}

type KnowledgeStatsHandler struct {
	graph  KnowledgeGraphQuerier
	vector KnowledgeVectorInfo
	names  knowledgeCollectionNamer
}

func NewKnowledgeStatsHandler(graph KnowledgeGraphQuerier, vector KnowledgeVectorInfo, names knowledgeCollectionNamer) *KnowledgeStatsHandler {
	return &KnowledgeStatsHandler{graph: graph, vector: vector, names: names}
}

func (h *KnowledgeStatsHandler) Stats(c *echo.Context) error {
	cid := ctxkey.MustCompanyID(c.Request().Context())
	ctx := c.Request().Context()

	return c.JSON(http.StatusOK, response.OK(map[string]any{
		"memgraph": h.memgraphStats(ctx, cid),
		"qdrant":   h.qdrantStats(ctx, cid),
	}))
}

func (h *KnowledgeStatsHandler) memgraphStats(ctx context.Context, companyID int64) map[string]any {
	unavailable := map[string]any{
		"available":  false,
		"nodes":      0,
		"edges":      0,
		"labels":     []map[string]any{},
		"edge_types": []map[string]any{},
	}
	if h.graph == nil {
		return unavailable
	}

	nodeRows, err := h.graph.Query(ctx, ports.CypherQuery{
		Statement: `
MATCH (n {company_id: $company_id})
RETURN labels(n)[0] AS label, count(n) AS c
ORDER BY c DESC`,
		Params: map[string]any{"company_id": companyID},
	})
	if err != nil {
		return unavailable
	}

	edgeRows, err := h.graph.Query(ctx, ports.CypherQuery{
		Statement: `
MATCH ()-[r {company_id: $company_id}]->()
RETURN type(r) AS edge_type, count(r) AS c
ORDER BY c DESC`,
		Params: map[string]any{"company_id": companyID},
	})
	if err != nil {
		return unavailable
	}

	labels := make([]map[string]any, 0, len(nodeRows))
	totalNodes := 0
	for _, row := range nodeRows {
		count := asInt(row["c"])
		totalNodes += count
		labels = append(labels, map[string]any{
			"label": asString(row["label"]),
			"count": count,
		})
	}

	edgeTypes := make([]map[string]any, 0, len(edgeRows))
	totalEdges := 0
	for _, row := range edgeRows {
		count := asInt(row["c"])
		totalEdges += count
		edgeTypes = append(edgeTypes, map[string]any{
			"type":  asString(row["edge_type"]),
			"count": count,
		})
	}

	return map[string]any{
		"available":  true,
		"nodes":      totalNodes,
		"edges":      totalEdges,
		"labels":     labels,
		"edge_types": edgeTypes,
	}
}

func (h *KnowledgeStatsHandler) qdrantStats(ctx context.Context, companyID int64) map[string]any {
	unavailable := map[string]any{
		"available":  false,
		"collection": "",
		"vectors":    0,
		"dim":        0,
	}
	if h.vector == nil || h.names == nil {
		return unavailable
	}

	collection, err := h.names.CollectionForCompany(companyID)
	if err != nil {
		return unavailable
	}

	points, dim, exists, err := h.vector.CollectionInfo(ctx, collection)
	if err != nil || !exists {
		return unavailable
	}

	return map[string]any{
		"available":  true,
		"collection": collection,
		"vectors":    points,
		"dim":        dim,
	}
}

func (h *KnowledgeStatsHandler) Graph(c *echo.Context) error {
	cid := ctxkey.MustCompanyID(c.Request().Context())
	limit := clampGraphLimit(parseInt(c.QueryParam("limit"), 80))
	ctx := c.Request().Context()

	unavailable := map[string]any{
		"available": false,
		"nodes":     []map[string]any{},
		"edges":     []map[string]any{},
	}

	if h.graph == nil {
		return c.JSON(http.StatusOK, response.OK(unavailable))
	}

	nodeRows, err := h.graph.Query(ctx, ports.CypherQuery{
		Statement: `
MATCH (n {company_id: $company_id})
OPTIONAL MATCH (n)-[r {company_id: $company_id}]-()
WITH n, count(r) AS degree
RETURN n.id AS id, coalesce(n.title, n.name, n.id) AS label, labels(n)[0] AS type, degree
ORDER BY degree DESC
LIMIT $limit`,
		Params: map[string]any{"company_id": cid, "limit": limit},
	})
	if err != nil {
		return c.JSON(http.StatusOK, response.OK(unavailable))
	}

	ids := make([]string, 0, len(nodeRows))
	nodes := make([]map[string]any, 0, len(nodeRows))
	for _, row := range nodeRows {
		id := asString(row["id"])
		ids = append(ids, id)
		nodes = append(nodes, map[string]any{
			"id":     id,
			"label":  asString(row["label"]),
			"type":   asString(row["type"]),
			"degree": asInt(row["degree"]),
		})
	}

	edges := make([]map[string]any, 0)
	if len(ids) > 0 {
		edgeRows, err := h.graph.Query(ctx, ports.CypherQuery{
			Statement: `
MATCH (a {company_id: $company_id})-[r {company_id: $company_id}]->(b {company_id: $company_id})
WHERE a.id IN $ids AND b.id IN $ids
RETURN a.id AS from, b.id AS to, type(r) AS type`,
			Params: map[string]any{"company_id": cid, "ids": ids},
		})
		if err == nil {
			for _, row := range edgeRows {
				edges = append(edges, map[string]any{
					"from": asString(row["from"]),
					"to":   asString(row["to"]),
					"type": asString(row["type"]),
				})
			}
		}
	}

	return c.JSON(http.StatusOK, response.OK(map[string]any{
		"available": true,
		"nodes":     nodes,
		"edges":     edges,
	}))
}

func clampGraphLimit(limit int) int {
	if limit < 10 {
		return 10
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
