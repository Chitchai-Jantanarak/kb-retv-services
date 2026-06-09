package mysql

import (
	"context"
	"errors"
	"fmt"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/tenant"
)

type FeedbackRecorder struct {
	db tenant.Querier
}

func NewFeedbackRecorder(db tenant.Querier) *FeedbackRecorder {
	return &FeedbackRecorder{db: db}
}

func (r *FeedbackRecorder) RecordFeedback(ctx context.Context, fb ports.AIActionFeedback) error {
	if fb.CompanyID <= 0 {
		return errors.New("ai_actions feedback: company_id must be positive")
	}
	if fb.ActionID <= 0 {
		return errors.New("ai_actions feedback: action id must be positive")
	}

	res, err := r.db.ExecContext(ctx, `
UPDATE ai_actions
SET feedback_score = ?
WHERE id = ? AND company_id = ?`, fb.Score, fb.ActionID, fb.CompanyID)
	if err != nil {
		return fmt.Errorf("ai_actions feedback: update: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("ai_actions feedback: action %d not found for company %d", fb.ActionID, fb.CompanyID)
	}
	return nil
}

var _ ports.AIActionFeedbackRecorder = (*FeedbackRecorder)(nil)
