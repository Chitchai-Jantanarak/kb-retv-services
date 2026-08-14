package mysql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/my/app/internal/infra/tenant"
)

type Repository struct {
	db tenant.Querier
}

func New(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) UpsertNonce(ctx context.Context, companyID int64, lineUserID, nonce string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO line_links (company_id, line_user_id, nonce, nonce_expires_at)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE nonce = VALUES(nonce), nonce_expires_at = VALUES(nonce_expires_at)`,
		companyID, lineUserID, nonce, expiresAt)
	return err
}

func (r *Repository) LinkByNonce(ctx context.Context, companyID int64, nonce string, customerID int64, now time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE line_links
		SET customer_id = ?, linked_at = ?, nonce = NULL, nonce_expires_at = NULL
		WHERE company_id = ? AND nonce = ? AND nonce_expires_at > ?`,
		customerID, now, companyID, nonce, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *Repository) CustomerFor(ctx context.Context, companyID int64, lineUserID string) (int64, bool, error) {
	var id sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT customer_id FROM line_links
		WHERE company_id = ? AND line_user_id = ? AND linked_at IS NOT NULL`,
		companyID, lineUserID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if !id.Valid || id.Int64 <= 0 {
		return 0, false, nil
	}
	return id.Int64, true, nil
}
