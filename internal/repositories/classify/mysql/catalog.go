package mysql

import (
	"context"

	"github.com/my/app/internal/ai/symptom"
	"github.com/my/app/internal/infra/tenant"
	"github.com/my/app/internal/shared/ctxkey"
)

type SymptomCatalog struct {
	db    tenant.Querier
	limit int
}

func NewSymptomCatalog(db tenant.Querier) *SymptomCatalog {
	return &SymptomCatalog{db: db, limit: 100}
}

func (c *SymptomCatalog) Symptoms(ctx context.Context, companyID int64) ([]symptom.Symptom, error) {
	if companyID <= 0 {
		return nil, nil
	}
	ctx = ctxkey.WithCompanyID(ctx, companyID)

	limit := c.limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := c.db.QueryContext(ctx, `
SELECT id, name
FROM symptom_nodes
WHERE company_id = ? AND status = 'active'
ORDER BY occurrence_count DESC, id ASC
LIMIT ?`, companyID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]symptom.Symptom, 0, limit)
	for rows.Next() {
		var s symptom.Symptom
		if err := rows.Scan(&s.ID, &s.Name); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

var _ symptom.Catalog = (*SymptomCatalog)(nil)
