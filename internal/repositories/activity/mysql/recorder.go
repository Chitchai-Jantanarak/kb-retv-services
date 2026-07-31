package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/tenant"
)

type Recorder struct {
	db tenant.Querier
}

func New(db tenant.Querier) *Recorder {
	return &Recorder{db: db}
}

func (r *Recorder) RecordActivity(ctx context.Context, e ports.ActivityEntry) error {
	if e.CompanyID <= 0 {
		return errors.New("activity_log: company_id must be positive")
	}
	if strings.TrimSpace(e.ActorType) == "" {
		return errors.New("activity_log: actor_type is required")
	}
	if strings.TrimSpace(e.Action) == "" {
		return errors.New("activity_log: action is required")
	}

	contextJSON, err := encodeContext(e.Context)
	if err != nil {
		return fmt.Errorf("activity_log: encode context: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
INSERT INTO activity_log
  (company_id, actor_type, actor_id, actor_label, actor_external,
   subject_type, subject_id, subject_label, action, changes, context, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, NOW())`,
		e.CompanyID,
		strings.TrimSpace(e.ActorType),
		nullableID(e.ActorID),
		nullableText(e.ActorLabel),
		nullableText(e.ActorExternal),
		nullableText(e.SubjectType),
		nullableID(e.SubjectID),
		nullableText(e.SubjectLabel),
		strings.TrimSpace(e.Action),
		contextJSON)
	if err != nil {
		return fmt.Errorf("activity_log: insert: %w", err)
	}
	return nil
}

func encodeContext(v map[string]any) (any, error) {
	if len(v) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func nullableID(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func nullableText(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

var _ ports.ActivityRecorder = (*Recorder)(nil)
