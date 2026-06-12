package mysql

import (
	"context"
	"database/sql"
	"strings"

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
WHERE ch.id > ? AND ch.chunk_kind = 'child'
  AND (emb.id IS NULL OR emb.content_hash IS NULL OR emb.content_hash <> ch.content_hash)`
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
WHERE ch.id > ? AND ch.chunk_kind = 'child'
  AND (emb.id IS NULL OR emb.content_hash IS NULL OR emb.content_hash <> ch.content_hash)`
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

func (s *ChunkSource) MetaByIDs(ctx context.Context, companyID int64, ids []int64) ([]vectorindex.Chunk, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if companyID > 0 {
		ctx = ctxkey.WithCompanyID(ctx, companyID)
	}

	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, companyID)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := `
SELECT
  ch.id,
  ch.kb_article_id,
  a.company_id,
  COALESCE(a.source_report_id, 0),
  COALESCE(ch.chunk_index, 0),
  ''
FROM kb_chunks ch
JOIN kb_articles a ON a.id = ch.kb_article_id
WHERE a.company_id = ? AND ch.id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanChunks(rows, len(ids))
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
INSERT INTO kb_embeddings (kb_chunk_id, vector_ref, model, dimensions, content_hash)
VALUES (?, ?, ?, ?, (SELECT content_hash FROM kb_chunks WHERE id = ?))
ON DUPLICATE KEY UPDATE
  vector_ref = VALUES(vector_ref),
  dimensions = VALUES(dimensions),
  content_hash = VALUES(content_hash)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for _, ref := range refs {
		if _, err := stmt.ExecContext(ctx, ref.ChunkID, ref.VectorRef, ref.Model, ref.Dim, ref.ChunkID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var (
	_ vectorindex.ChunkSource = (*ChunkSource)(nil)
	_ vectorindex.MetaSource  = (*ChunkSource)(nil)
	_ vectorindex.Marker      = (*EmbeddingMarker)(nil)
)
