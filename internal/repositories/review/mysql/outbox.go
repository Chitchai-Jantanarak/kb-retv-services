package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/domain/ports"
)

type OutboxWriter struct {
	db *sql.DB
}

func NewOutboxWriter(db *sql.DB) *OutboxWriter {
	return &OutboxWriter{db: db}
}

func (w *OutboxWriter) Enqueue(ctx context.Context, item ports.ReviewItem) error {
	if item.CompanyID <= 0 {
		return errors.New("ai_review_outbox: company_id must be positive")
	}
	kind := strings.TrimSpace(string(item.Kind))
	if kind == "" {
		return errors.New("ai_review_outbox: kind is required")
	}
	if item.Payload == nil {
		return errors.New("ai_review_outbox: payload is required")
	}

	payloadJSON, err := json.Marshal(item.Payload)
	if err != nil {
		return fmt.Errorf("ai_review_outbox: marshal: %w", err)
	}
	sum := sha256.Sum256(payloadJSON)
	hash := hex.EncodeToString(sum[:])

	_, err = w.db.ExecContext(ctx, `
INSERT INTO ai_review_outbox
  (company_id, kind, payload, payload_hash, status, attempts, created_at)
VALUES
  (?, ?, ?, ?, 'pending', 0, NOW())
ON DUPLICATE KEY UPDATE
  payload  = VALUES(payload),
  status   = IF(status='failed', 'pending', status),
  attempts = attempts`, item.CompanyID, kind, payloadJSON, hash)
	if err != nil {
		return fmt.Errorf("ai_review_outbox: insert: %w", err)
	}
	return nil
}

func (w *OutboxWriter) PendingReviewOutbox(ctx context.Context, companyID int64, limit int) ([]ports.ReviewOutboxItem, error) {
	if companyID <= 0 {
		return nil, errors.New("ai_review_outbox: company_id must be positive")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := w.db.QueryContext(ctx, `
SELECT id, company_id, kind, payload, payload_hash, attempts
FROM ai_review_outbox
WHERE company_id = ? AND status IN ('pending', 'failed') AND attempts < 10
ORDER BY created_at ASC, id ASC
LIMIT ?`, companyID, limit)
	if err != nil {
		return nil, fmt.Errorf("ai_review_outbox: list pending: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]ports.ReviewOutboxItem, 0)
	for rows.Next() {
		var item ports.ReviewOutboxItem
		var kind string
		var payload []byte
		if err := rows.Scan(&item.ID, &item.CompanyID, &kind, &payload, &item.PayloadHash, &item.Attempts); err != nil {
			return nil, fmt.Errorf("ai_review_outbox: scan pending: %w", err)
		}
		item.Kind = ports.ReviewKind(kind)
		item.Payload = append(item.Payload, payload...)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ai_review_outbox: rows: %w", err)
	}
	return items, nil
}

func (w *OutboxWriter) MarkReviewOutboxSent(ctx context.Context, id int64, laravelRef int64) error {
	if id <= 0 {
		return errors.New("ai_review_outbox: id must be positive")
	}
	_, err := w.db.ExecContext(ctx, `
UPDATE ai_review_outbox
SET status = 'sent', laravel_ref = ?, attempts = attempts + 1, last_error = NULL
WHERE id = ?`, nullableLaravelRef(laravelRef), id)
	if err != nil {
		return fmt.Errorf("ai_review_outbox: mark sent: %w", err)
	}
	return nil
}

func (w *OutboxWriter) MarkReviewOutboxFailed(ctx context.Context, id int64, reason string) error {
	if id <= 0 {
		return errors.New("ai_review_outbox: id must be positive")
	}
	_, err := w.db.ExecContext(ctx, `
UPDATE ai_review_outbox
SET status = 'failed', attempts = attempts + 1, last_error = ?
WHERE id = ?`, truncateOutboxError(reason), id)
	if err != nil {
		return fmt.Errorf("ai_review_outbox: mark failed: %w", err)
	}
	return nil
}

func nullableLaravelRef(ref int64) any {
	if ref <= 0 {
		return nil
	}
	return ref
}

func truncateOutboxError(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) > 1000 {
		return reason[:1000]
	}
	return reason
}

var (
	_ ports.ReviewQueueWriter = (*OutboxWriter)(nil)
	_ ports.ReviewOutboxStore = (*OutboxWriter)(nil)
)
