package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/my/app/internal/application/workflows/promote"
	"github.com/my/app/internal/infra/tenant"
)

type ReviewRepository struct {
	db tenant.Querier
}

func NewReviewRepository(db tenant.Querier) *ReviewRepository {
	return &ReviewRepository{db: db}
}

func (r *ReviewRepository) Get(ctx context.Context, id int64) (promote.ReviewItem, error) {
	if id <= 0 {
		return promote.ReviewItem{}, errors.New("review_queue: id must be positive")
	}
	var (
		companyID int64
		kind      string
		payload   sql.NullString
	)
	err := r.db.QueryRowContext(ctx, `
SELECT company_id, kind, COALESCE(edited_payload, payload)
FROM review_queue
WHERE id = ? AND status = 'pending'`, id).Scan(&companyID, &kind, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return promote.ReviewItem{}, fmt.Errorf("review_queue: id %d not pending", id)
	}
	if err != nil {
		return promote.ReviewItem{}, fmt.Errorf("review_queue: load %d: %w", id, err)
	}
	item := promote.ReviewItem{
		ID:        id,
		CompanyID: companyID,
		Kind:      promote.Kind(kind),
	}
	if payload.Valid {
		if err := json.Unmarshal([]byte(payload.String), &item.Payload); err != nil {
			return promote.ReviewItem{}, fmt.Errorf("review_queue: parse payload: %w", err)
		}
	}
	return item, nil
}

func (r *ReviewRepository) MarkApproved(ctx context.Context, id int64, promotionRef string) error {
	if id <= 0 {
		return errors.New("review_queue: id must be positive")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE review_queue
SET status = 'approved', reviewed_at = NOW(), promotion_ref = ?
WHERE id = ? AND status = 'pending'`, promotionRef, id)
	if err != nil {
		return fmt.Errorf("review_queue: mark approved %d: %w", id, err)
	}
	return nil
}

func (r *ReviewRepository) ActivateSymptom(ctx context.Context, companyID, symptomNodeID int64) error {
	if companyID <= 0 || symptomNodeID <= 0 {
		return errors.New("review_queue: company_id + symptom_node_id required")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE symptom_nodes
SET status = 'active'
WHERE id = ? AND company_id = ?`, symptomNodeID, companyID)
	if err != nil {
		return fmt.Errorf("review_queue: activate symptom %d: %w", symptomNodeID, err)
	}
	return nil
}

func (r *ReviewRepository) MarkRejected(ctx context.Context, companyID, id int64, reason string) error {
	if id <= 0 || companyID <= 0 {
		return errors.New("review_queue: id + company_id required")
	}
	res, err := r.db.ExecContext(ctx, `
UPDATE review_queue
SET status = 'rejected', reviewed_at = NOW(), review_notes = ?
WHERE id = ? AND company_id = ? AND status = 'pending'`, reason, id, companyID)
	if err != nil {
		return fmt.Errorf("review_queue: mark rejected %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("review_queue: id %d not pending for company %d", id, companyID)
	}
	return nil
}

func (r *ReviewRepository) ActivateSubject(ctx context.Context, companyID, subjectNodeID int64) error {
	if companyID <= 0 || subjectNodeID <= 0 {
		return errors.New("review_queue: company_id + subject_node_id required")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE subject_nodes
SET status = 'active'
WHERE id = ? AND company_id = ?`, subjectNodeID, companyID)
	if err != nil {
		return fmt.Errorf("review_queue: activate subject %d: %w", subjectNodeID, err)
	}
	return nil
}

var _ promote.Repo = (*ReviewRepository)(nil)
