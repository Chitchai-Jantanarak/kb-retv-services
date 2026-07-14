package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/my/app/internal/application/profile"
	"github.com/my/app/internal/infra/tenant"
)

const (
	maxProducts     = 30
	maxProblemTypes = 30
)

type Repository struct {
	db tenant.Querier
}

func New(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Load(ctx context.Context, companyID int64) (profile.Data, error) {
	if companyID <= 0 {
		return profile.Data{}, errors.New("profile: company_id must be positive")
	}

	var data profile.Data
	var name, website, companysite, tone, signature sql.NullString

	err := r.db.QueryRowContext(ctx, `
SELECT
  c.name,
  c.website,
  c.companysite,
  b.ai_reply_tone,
  b.ai_signature
FROM companies c
LEFT JOIN company_branding b ON b.company_id = c.id
WHERE c.id = ?
LIMIT 1`, companyID).Scan(&name, &website, &companysite, &tone, &signature)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return profile.Data{}, fmt.Errorf("profile: load identity: %w", err)
	}
	data.Name = name.String
	data.Website = website.String
	data.CompanySite = companysite.String
	data.ReplyTone = tone.String
	data.Signature = signature.String

	products, err := r.stringColumn(ctx, `
SELECT name
FROM subject_nodes
WHERE company_id = ? AND status = 'active'
ORDER BY sort_order ASC, id ASC
LIMIT ?`, companyID, maxProducts)
	if err != nil {
		return profile.Data{}, fmt.Errorf("profile: load products: %w", err)
	}
	data.Products = products

	types, err := r.stringColumn(ctx, `
SELECT COALESCE(NULLIF(cpt.custom_label, ''), pt.name)
FROM company_problem_types cpt
JOIN problem_types pt ON pt.id = cpt.problem_type_id
WHERE cpt.company_id = ? AND cpt.is_active = 1
ORDER BY pt.sort_order ASC, pt.id ASC
LIMIT ?`, companyID, maxProblemTypes)
	if err != nil {
		return profile.Data{}, fmt.Errorf("profile: load problem types: %w", err)
	}
	data.ProblemTypes = types

	return data, nil
}

func (r *Repository) stringColumn(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]string, 0)
	for rows.Next() {
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		if v.Valid {
			out = append(out, v.String)
		}
	}
	return out, rows.Err()
}

var _ profile.Repository = (*Repository)(nil)
