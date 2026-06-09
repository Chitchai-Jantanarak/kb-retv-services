package memgraph

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/my/app/internal/domain/ports"
)

// Store implements ports.GraphStore against a Memgraph (Bolt) endpoint.
// Memgraph is wire-compatible with neo4j-go-driver v5.
type Store struct {
	driver neo4j.DriverWithContext
}

type Config struct {
	URI      string
	Username string
	Password string
}

func NewStore(cfg Config) (*Store, error) {
	auth := neo4j.NoAuth()
	if cfg.Username != "" {
		auth = neo4j.BasicAuth(cfg.Username, cfg.Password, "")
	}
	driver, err := neo4j.NewDriverWithContext(cfg.URI, auth)
	if err != nil {
		return nil, fmt.Errorf("memgraph: new driver: %w", err)
	}
	return &Store{driver: driver}, nil
}

func (s *Store) Close(ctx context.Context) error {
	return s.driver.Close(ctx)
}

// VerifyConnectivity checks that the Bolt connection is alive.
func (s *Store) VerifyConnectivity(ctx context.Context) error {
	return s.driver.VerifyConnectivity(ctx)
}

// UpsertNode merges a node by (company_id, id) and sets all props.
// The first label in n.Labels is used as the primary label.
func (s *Store) UpsertNode(ctx context.Context, n ports.Node) error {
	if n.CompanyID <= 0 {
		return errors.New("memgraph: UpsertNode: company_id must be positive")
	}
	if len(n.Labels) == 0 {
		return errors.New("memgraph: UpsertNode: node must have at least one label")
	}
	label := n.Labels[0]

	props := make(map[string]any, len(n.Props)+2)
	maps.Copy(props, n.Props)
	props["id"] = n.ID
	props["company_id"] = n.CompanyID

	cypher := fmt.Sprintf(`
		MERGE (n:%s { company_id: $company_id, id: $id })
		SET n += $props
	`, label)

	return s.run(ctx, cypher, map[string]any{
		"company_id": n.CompanyID,
		"id":         n.ID,
		"props":      props,
	})
}

// UpsertEdge merges an edge between two nodes scoped to the same company.
// Both endpoints and the relationship carry company_id; cross-tenant MATCH
// cannot resolve because UpsertNode merges on (company_id, id).
func (s *Store) UpsertEdge(ctx context.Context, e ports.Edge) error {
	if e.CompanyID <= 0 {
		return errors.New("memgraph: UpsertEdge: company_id must be positive")
	}

	cypher := fmt.Sprintf(`
		MATCH (a { company_id: $company_id, id: $from })
		MATCH (b { company_id: $company_id, id: $to })
		MERGE (a)-[r:%s { company_id: $company_id }]->(b)
		SET r += $props
	`, e.Type)

	props := make(map[string]any, len(e.Props)+1)
	maps.Copy(props, e.Props)
	props["company_id"] = e.CompanyID

	return s.run(ctx, cypher, map[string]any{
		"company_id": e.CompanyID,
		"from":       e.From,
		"to":         e.To,
		"props":      props,
	})
}

// Query runs an arbitrary Cypher statement and returns all rows.
func (s *Store) Query(ctx context.Context, q ports.CypherQuery) ([]map[string]any, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer func() { _ = session.Close(ctx) }()

	result, err := session.Run(ctx, q.Statement, q.Params)
	if err != nil {
		return nil, fmt.Errorf("memgraph: query: %w", err)
	}

	var rows []map[string]any
	for result.Next(ctx) {
		row := make(map[string]any)
		for _, key := range result.Record().Keys {
			row[key] = result.Record().AsMap()[key]
		}
		rows = append(rows, row)
	}
	if err := result.Err(); err != nil {
		return nil, fmt.Errorf("memgraph: query rows: %w", err)
	}
	return rows, nil
}

func (s *Store) run(ctx context.Context, cypher string, params map[string]any) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func() { _ = session.Close(ctx) }()

	_, err := session.Run(ctx, cypher, params)
	if err != nil {
		return fmt.Errorf("memgraph: run: %w", err)
	}
	return nil
}

var _ ports.GraphStore = (*Store)(nil)
