package rag

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/infra/tenant"
)

type FTSChunk struct {
	ChunkID   int64
	ArticleID int64
	Title     string
	Content   string
	Relevance float64
}

type FTSSource interface {
	SearchChunks(ctx context.Context, companyID int64, query string, limit int) ([]FTSChunk, error)
}

type FTSRetriever struct {
	source FTSSource
}

func NewFTSRetriever(source FTSSource) (*FTSRetriever, error) {
	if source == nil {
		return nil, errors.New("rag: fts retriever: source is required")
	}
	return &FTSRetriever{source: source}, nil
}

func (r *FTSRetriever) Retrieve(ctx context.Context, query Query, _ Meta) ([]Candidate, error) {
	q := strings.TrimSpace(query.Text)
	if q == "" {
		return nil, nil
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 5
	}
	rows, err := r.source.SearchChunks(ctx, query.CompanyID, q, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, Candidate{
			ID:        fmt.Sprintf("chunk:%d", row.ChunkID),
			ArticleID: fmt.Sprintf("%d", row.ArticleID),
			ChunkID:   fmt.Sprintf("%d", row.ChunkID),
			Title:     row.Title,
			Content:   row.Content,
			Source:    "fts",
			Score:     row.Relevance,
		})
	}
	return out, nil
}

func (r *FTSRetriever) RetrievalCost() RetrievalCost {
	return RetrievalCostCheap
}

var _ Retriever = (*FTSRetriever)(nil)
var _ CostAwareRetriever = (*FTSRetriever)(nil)

type MySQLFTSSource struct {
	db tenant.Querier
}

func NewMySQLFTSSource(db tenant.Querier) *MySQLFTSSource {
	return &MySQLFTSSource{db: db}
}

func (s *MySQLFTSSource) SearchChunks(ctx context.Context, companyID int64, query string, limit int) ([]FTSChunk, error) {
	if companyID <= 0 {
		return nil, errors.New("kb_chunks FTS: company_id must be positive")
	}
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  ch.id,
  ch.kb_article_id,
  COALESCE(a.title, ''),
  COALESCE(p.content, ch.content) AS context_content,
  MATCH(ch.content) AGAINST(? IN NATURAL LANGUAGE MODE) AS relevance
FROM kb_chunks ch
JOIN kb_articles a ON a.id = ch.kb_article_id
LEFT JOIN kb_chunks p ON p.id = ch.parent_chunk_id
WHERE a.company_id = ?
  AND ch.chunk_kind = 'child'
  AND MATCH(ch.content) AGAINST(? IN NATURAL LANGUAGE MODE)
ORDER BY relevance DESC, ch.id ASC
LIMIT ?`, query, companyID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("kb_chunks FTS: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]FTSChunk, 0, limit)
	for rows.Next() {
		var c FTSChunk
		if err := rows.Scan(&c.ChunkID, &c.ArticleID, &c.Title, &c.Content, &c.Relevance); err != nil {
			return nil, fmt.Errorf("kb_chunks FTS: scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

var _ FTSSource = (*MySQLFTSSource)(nil)
