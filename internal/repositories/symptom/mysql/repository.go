package mysql

import (
	"context"
	"errors"
	"fmt"

	"github.com/my/app/internal/application/workflows/symptommerge"
	"github.com/my/app/internal/infra/tenant"
)

type Repository struct {
	db tenant.Querier
}

func New(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) UnmergedSymptoms(ctx context.Context, companyID int64) ([]symptommerge.SymptomRow, error) {
	if companyID <= 0 {
		return nil, errors.New("symptom repo: company_id must be positive")
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, name, occurrence_count
FROM symptom_nodes
WHERE company_id = ? AND canonical_id IS NULL AND status IN ('active','proposed') AND name <> ''
ORDER BY occurrence_count DESC, id ASC`, companyID)
	if err != nil {
		return nil, fmt.Errorf("symptom repo: list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []symptommerge.SymptomRow{}
	for rows.Next() {
		var row symptommerge.SymptomRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Count); err != nil {
			return nil, fmt.Errorf("symptom repo: scan: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repository) SetCanonical(ctx context.Context, companyID, id, canonicalID int64) error {
	if id == canonicalID {
		return errors.New("symptom repo: symptom cannot be its own canonical")
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE symptom_nodes SET canonical_id = ?
WHERE company_id = ? AND id = ?`, canonicalID, companyID, id)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
UPDATE symptom_nodes SET canonical_id = ?
WHERE company_id = ? AND canonical_id = ?`, canonicalID, companyID, id)
	return err
}
