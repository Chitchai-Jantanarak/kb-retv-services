package mysql

import (
	"context"
	"database/sql"

	"github.com/my/app/internal/application/services/vectorindex"
	"github.com/my/app/internal/infra/tenant"
	"github.com/my/app/internal/shared/ctxkey"
)

type ChunkSource struct {
	db    tenant.Querier
	model string
}

type EmbeddingMarker struct {
	db tenant.Querier
}

func NewChunkSource(db tenant.Querier) *ChunkSource {
	return &ChunkSource{db: db}
}

func NewChunkSourceForModel(db tenant.Querier, model string) *ChunkSource {
	return &ChunkSource{db: db, model: model}
}

func NewEmbeddingMarker(db tenant.Querier) *EmbeddingMarker {
	return &EmbeddingMarker{db: db}
}

func (s *ChunkSource) Next(ctx context.Context, companyID int64, afterID int64, limit int) ([]vectorindex.Chunk, error) {
	if limit <= 0 {
		limit = 64
	}
	if companyID > 0 {
		ctx = ctxkey.WithCompanyID(ctx, companyID)
	}

	if s.model == "" {
		return s.nextWithoutAnyEmbedding(ctx, companyID, afterID, limit)
	}
	return s.nextWithoutModelEmbedding(ctx, companyID, afterID, limit)
}

func (s *ChunkSource) nextWithoutAnyEmbedding(ctx context.Context, companyID int64, afterID int64, limit int) ([]vectorindex.Chunk, error) {
	query := `
SELECT
  ch.id,
  ch.kb_article_id,
  a.company_id,
  COALESCE(a.source_report_id, 0),
  COALESCE(ch.chunk_index, 0),
  COALESCE(ch.content, '')
FROM kb_chunks ch
JOIN kb_articles a ON a.id = ch.kb_article_id
LEFT JOIN kb_embeddings emb ON emb.kb_chunk_id = ch.id
WHERE ch.id > ? AND emb.id IS NULL AND ch.chunk_kind = 'child'`
	args := []any{afterID}
	if companyID > 0 {
		query += " AND a.company_id = ?"
		args = append(args, companyID)
	}
	query += "\nORDER BY ch.id ASC\nLIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanChunks(rows, limit)
}

func (s *ChunkSource) nextWithoutModelEmbedding(ctx context.Context, companyID int64, afterID int64, limit int) ([]vectorindex.Chunk, error) {
	query := `
SELECT
  ch.id,
  ch.kb_article_id,
  a.company_id,
  COALESCE(a.source_report_id, 0),
  COALESCE(ch.chunk_index, 0),
  COALESCE(ch.content, '')
FROM kb_chunks ch
JOIN kb_articles a ON a.id = ch.kb_article_id
LEFT JOIN kb_embeddings emb ON emb.kb_chunk_id = ch.id AND emb.model = ?
WHERE ch.id > ? AND emb.id IS NULL AND ch.chunk_kind = 'child'`
	args := []any{s.model, afterID}
	if companyID > 0 {
		query += " AND a.company_id = ?"
		args = append(args, companyID)
	}
	query += "\nORDER BY ch.id ASC\nLIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanChunks(rows, limit)
}

func scanChunks(rows *sql.Rows, limit int) ([]vectorindex.Chunk, error) {
	chunks := make([]vectorindex.Chunk, 0, limit)
	for rows.Next() {
		var chunk vectorindex.Chunk
		if err := rows.Scan(
			&chunk.ID,
			&chunk.ArticleID,
			&chunk.CompanyID,
			&chunk.SourceReportID,
			&chunk.ChunkIndex,
			&chunk.Content,
		); err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return chunks, nil
}

func (m *EmbeddingMarker) Mark(ctx context.Context, refs []vectorindex.ChunkRef) error {
	if len(refs) == 0 {
		return nil
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO kb_embeddings (kb_chunk_id, vector_ref, model, dimensions)
VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  vector_ref = VALUES(vector_ref),
  dimensions = VALUES(dimensions)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for _, ref := range refs {
		if _, err := stmt.ExecContext(ctx, ref.ChunkID, ref.VectorRef, ref.Model, ref.Dim); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var (
	_ vectorindex.ChunkSource = (*ChunkSource)(nil)
	_ vectorindex.Marker      = (*EmbeddingMarker)(nil)
)
