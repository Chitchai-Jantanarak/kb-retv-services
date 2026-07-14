package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/my/app/internal/application/workflows/intake"
	"github.com/my/app/internal/infra/tenant"
)

type SpecRepository struct {
	db tenant.Querier
}

func NewSpecRepository(db tenant.Querier) *SpecRepository {
	return &SpecRepository{db: db}
}

func (r *SpecRepository) SpecFor(ctx context.Context, companyID int64) (intake.Spec, error) {
	if companyID <= 0 {
		return intake.Spec{}, errors.New("intake spec: company_id must be positive")
	}

	var path sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT path FROM companies WHERE id = ? LIMIT 1`, companyID).Scan(&path)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return intake.Spec{}, fmt.Errorf("intake spec: load company path: %w", err)
	}

	for _, ancestorID := range intake.Lineage(path.String, companyID, intake.LineageMaxAncestors) {
		defs, derr := r.definitions(ctx, ancestorID)
		if derr != nil {
			return intake.Spec{}, derr
		}
		if spec, custom := intake.SpecFromDefinitions(defs); custom {
			return spec, nil
		}
	}

	return intake.DefaultSpec(), nil
}

func (r *SpecRepository) definitions(ctx context.Context, companyID int64) ([]intake.FieldDefinition, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT field_key, (is_required = 1 OR required = 1) AS is_required
FROM custom_field_definitions
WHERE company_id = ?
  AND entity_type = ?
  AND enabled = 1
  AND is_active = 1
  AND field_key IS NOT NULL
  AND field_key <> ''
ORDER BY sort_order ASC, display_order ASC, id ASC`, companyID, intake.EntityType)
	if err != nil {
		return nil, fmt.Errorf("intake spec: load definitions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	defs := make([]intake.FieldDefinition, 0)
	for rows.Next() {
		var (
			key      sql.NullString
			required sql.NullBool
		)
		if err := rows.Scan(&key, &required); err != nil {
			return nil, fmt.Errorf("intake spec: scan definition: %w", err)
		}
		defs = append(defs, intake.FieldDefinition{Key: key.String, Required: required.Bool})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("intake spec: iterate definitions: %w", err)
	}
	return defs, nil
}

var _ intake.SpecResolver = (*SpecRepository)(nil)
