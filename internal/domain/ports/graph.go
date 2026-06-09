package ports

import "context"

type Node struct {
	CompanyID int64
	ID        string
	Labels    []string
	Props     map[string]any
}

type Edge struct {
	CompanyID int64
	From      string
	To        string
	Type      string
	Props     map[string]any
}

type CypherQuery struct {
	Statement string
	Params    map[string]any
}

type GraphStore interface {
	UpsertNode(ctx context.Context, n Node) error
	UpsertEdge(ctx context.Context, e Edge) error
	Query(ctx context.Context, q CypherQuery) ([]map[string]any, error)
}
