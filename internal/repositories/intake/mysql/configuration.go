package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/application/workflows/intake"
	"github.com/my/app/internal/infra/tenant"
)

// ConfigurationRepository reads mail policy, instructions, and /tree choices.
type ConfigurationRepository struct{ db tenant.Querier }

// NewConfigurationRepository uses the same tenant router as intake persistence.
func NewConfigurationRepository(db tenant.Querier) *ConfigurationRepository {
	return &ConfigurationRepository{db: db}
}

// ConfigurationFor keeps existing processing enabled until a tenant opts out.
func (r *ConfigurationRepository) ConfigurationFor(ctx context.Context, companyID int64) (intake.Configuration, error) {
	if companyID <= 0 {
		return intake.Configuration{}, errors.New("intake configuration: company_id must be positive")
	}
	var raw, instructions sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT intake_settings FROM companies WHERE id = ? LIMIT 1`, companyID).Scan(&raw)
	if err != nil {
		return intake.Configuration{}, fmt.Errorf("intake configuration: load policy: %w", err)
	}
	policy := struct {
		EvaluationEnabled bool `json:"evaluation_enabled"`
		RulesEnabled      bool `json:"rules_enabled"`
		AIEnabled         bool `json:"ai_enabled"`
		AutoCreateEnabled bool `json:"auto_create_enabled"`
	}{EvaluationEnabled: true, RulesEnabled: true, AIEnabled: true, AutoCreateEnabled: true}
	if strings.TrimSpace(raw.String) != "" {
		if err := json.Unmarshal([]byte(raw.String), &policy); err != nil {
			return intake.Configuration{}, fmt.Errorf("intake configuration: decode policy: %w", err)
		}
	}
	err = r.db.QueryRowContext(ctx, `SELECT system_prompt FROM ai_agents WHERE company_id = ? AND is_active = 1 ORDER BY id LIMIT 1`, companyID).Scan(&instructions)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return intake.Configuration{}, fmt.Errorf("intake configuration: load instructions: %w", err)
	}
	return intake.Configuration{EvaluationDisabled: !policy.EvaluationEnabled, RulesDisabled: !policy.RulesEnabled, AIEnabled: policy.AIEnabled, AutoCreateEnabled: policy.AutoCreateEnabled, Instructions: instructions.String}, nil
}

// Products returns named choices below /tree roots, rather than treating root
// headings or AI-discovered subject nodes as the tenant's service catalogue.
func (r *ConfigurationRepository) Products(ctx context.Context, companyID int64) ([]string, error) {
	if companyID <= 0 {
		return nil, errors.New("intake catalogue: company_id must be positive")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT title FROM nodes WHERE company_id = ? AND parent_id IS NOT NULL ORDER BY id LIMIT 30`, companyID)
	if err != nil {
		return nil, fmt.Errorf("intake catalogue: load tree: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("intake catalogue: scan tree: %w", err)
		}
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names, rows.Err()
}
