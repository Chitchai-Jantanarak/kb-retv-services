package mysql

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/tenant"
)

type KnowledgeGapStore struct {
	db tenant.Querier
}

func NewKnowledgeGapStore(db tenant.Querier) *KnowledgeGapStore {
	return &KnowledgeGapStore{db: db}
}

func (s *KnowledgeGapStore) Record(ctx context.Context, companyID int64, queryText string) error {
	queryText = strings.TrimSpace(queryText)
	if companyID <= 0 {
		return errors.New("knowledge_gap: company_id must be positive")
	}
	if queryText == "" {
		return errors.New("knowledge_gap: query_text is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var id int64
	err = tx.QueryRowContext(ctx, `
SELECT id
FROM knowledge_gaps
WHERE company_id = ? AND query_text = ?
LIMIT 1`, companyID, queryText).Scan(&id)

	now := time.Now().UTC()
	if err == nil {
		if _, err := tx.ExecContext(ctx, `
UPDATE knowledge_gaps
SET occurrence_count = occurrence_count + 1,
    last_seen = ?
WHERE id = ?`, now, id); err != nil {
			return err
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO knowledge_gaps
  (company_id, query_text, occurrence_count, first_seen, last_seen, status)
VALUES
  (?, ?, 1, ?, ?, 'open')`, companyID, queryText, now, now); err != nil {
		return err
	}
	return tx.Commit()
}

var _ ports.KnowledgeGapRecorder = (*KnowledgeGapStore)(nil)
