package ports

import "context"

type VectorPoint struct {
	ID       string
	Vector   []float32
	Metadata map[string]any
}

type VectorHit struct {
	ID       string
	Score    float32
	Metadata map[string]any
}

type VectorSearch struct {
	Collection string
	Query      []float32
	Limit      int
	Filter     map[string]any
}

type VectorStore interface {
	EnsureCollection(ctx context.Context, name string, dim int) error
	Upsert(ctx context.Context, collection string, points []VectorPoint) error
	Search(ctx context.Context, q VectorSearch) ([]VectorHit, error)
	Delete(ctx context.Context, collection string, ids []string) error
}
