package mysql

import (
	"context"
	"database/sql"
)

const BatchStateIngested = "INGESTED"

type EmbeddingBatch struct {
	ID           int64
	BatchName    string
	Provider     string
	Model        string
	Dimensions   int
	DisplayName  string
	State        string
	RequestCount int
	IndexedCount int
	InputFile    string
	ResultFile   string
	Error        string
}

type EmbeddingBatchRepo struct {
	db *sql.DB
}

func NewEmbeddingBatchRepo(db *sql.DB) *EmbeddingBatchRepo {
	return &EmbeddingBatchRepo{db: db}
}

func (r *EmbeddingBatchRepo) Insert(ctx context.Context, b EmbeddingBatch) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO kb_embedding_batches
  (batch_name, provider, model, dimensions, display_name, state, request_count, indexed_count, input_file, result_file, error, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`,
		b.BatchName, b.Provider, b.Model, b.Dimensions, b.DisplayName, b.State,
		b.RequestCount, b.IndexedCount, nullString(b.InputFile), nullString(b.ResultFile), nullString(b.Error))
	return err
}

func (r *EmbeddingBatchRepo) ListActive(ctx context.Context) ([]EmbeddingBatch, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, batch_name, provider, model, dimensions, display_name, state, request_count, indexed_count,
  COALESCE(input_file,''), COALESCE(result_file,''), COALESCE(error,'')
FROM kb_embedding_batches
WHERE state <> ?
  AND state NOT LIKE '%FAILED%'
  AND state NOT LIKE '%EXPIRED%'
  AND state NOT LIKE '%CANCELLED%'
ORDER BY id ASC`, BatchStateIngested)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanBatches(rows)
}

func (r *EmbeddingBatchRepo) GetByName(ctx context.Context, name string) (EmbeddingBatch, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, batch_name, provider, model, dimensions, display_name, state, request_count, indexed_count,
  COALESCE(input_file,''), COALESCE(result_file,''), COALESCE(error,'')
FROM kb_embedding_batches
WHERE batch_name = ?
LIMIT 1`, name)
	if err != nil {
		return EmbeddingBatch{}, err
	}
	defer func() { _ = rows.Close() }()
	batches, err := scanBatches(rows)
	if err != nil {
		return EmbeddingBatch{}, err
	}
	if len(batches) == 0 {
		return EmbeddingBatch{}, sql.ErrNoRows
	}
	return batches[0], nil
}

func (r *EmbeddingBatchRepo) UpdateState(ctx context.Context, name, state, errMsg string) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE kb_embedding_batches SET state = ?, error = ?, updated_at = NOW() WHERE batch_name = ?`,
		state, nullString(errMsg), name)
	return err
}

func (r *EmbeddingBatchRepo) MarkIngested(ctx context.Context, name string, indexed int) error {
	_, err := r.db.ExecContext(ctx, `
UPDATE kb_embedding_batches SET state = ?, indexed_count = ?, error = NULL, updated_at = NOW() WHERE batch_name = ?`,
		BatchStateIngested, indexed, name)
	return err
}

func scanBatches(rows *sql.Rows) ([]EmbeddingBatch, error) {
	var out []EmbeddingBatch
	for rows.Next() {
		var b EmbeddingBatch
		if err := rows.Scan(
			&b.ID, &b.BatchName, &b.Provider, &b.Model, &b.Dimensions, &b.DisplayName,
			&b.State, &b.RequestCount, &b.IndexedCount, &b.InputFile, &b.ResultFile, &b.Error,
		); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
