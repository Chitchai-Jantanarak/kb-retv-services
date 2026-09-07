package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/infra/tenant"
)

type Repository struct {
	db tenant.Querier
}

func New(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) InstructionsFor(ctx context.Context, companyID int64) (string, error) {
	if companyID <= 0 {
		return "", errors.New("prompt: company_id must be positive")
	}

	var value sql.NullString
	err := r.db.QueryRowContext(ctx, `
SELECT system_prompt
FROM ai_agents
WHERE company_id = ? AND is_active = 1
ORDER BY (role = 'support_reply') DESC, updated_at DESC, id DESC
LIMIT 1`, companyID).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("prompt: load instructions: %w", err)
	}

	return strings.TrimSpace(value.String), nil
}
