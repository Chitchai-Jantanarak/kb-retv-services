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

type ActionRecorder struct {
	db tenant.Querier
}

func NewActionRecorder(db tenant.Querier) *ActionRecorder {
	return &ActionRecorder{db: db}
}

func (r *ActionRecorder) Record(ctx context.Context, a ports.AIAction) (int64, error) {
	if a.CompanyID <= 0 {
		return 0, errors.New("ai_actions: company_id must be positive")
	}
	if a.AgentID <= 0 {
		return 0, errors.New("ai_actions: ai_agent_id must be positive")
	}
	if strings.TrimSpace(a.ActionType) == "" {
		return 0, errors.New("ai_actions: action_type is required")
	}

	inputJSON, err := encodeJSON(a.Input)
	if err != nil {
		return 0, fmt.Errorf("ai_actions: encode input: %w", err)
	}
	outputJSON, err := encodeJSON(a.Output)
	if err != nil {
		return 0, fmt.Errorf("ai_actions: encode output: %w", err)
	}

	res, err := r.db.ExecContext(ctx, `
INSERT INTO ai_actions
  (company_id, ai_agent_id, action_type, input, output,
   tokens_in, tokens_out, latency_ms, confidence, error, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())`,
		a.CompanyID, a.AgentID, a.ActionType,
		inputJSON, outputJSON,
		nullableInt(a.TokensIn), nullableInt(a.TokensOut),
		nullableInt(a.LatencyMs), nullableFloatLocal(a.Confidence),
		nullableStr(a.Err))
	if err != nil {
		return 0, fmt.Errorf("ai_actions: insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("ai_actions: last insert id: %w", err)
	}
	return id, nil
}

func encodeJSON(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func nullableInt(v int) any {
	if v <= 0 {
		return nil
	}
	return v
}

func nullableFloatLocal(v float64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func nullableStr(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

var _ ports.AIActionRecorder = (*ActionRecorder)(nil)
