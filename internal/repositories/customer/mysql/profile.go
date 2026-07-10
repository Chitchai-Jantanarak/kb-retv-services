package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func buildProfileByNameQuery(coverage []int64, name string) (string, []any) {
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+1)
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, "%"+name+"%")
	query := fmt.Sprintf(`
SELECT
  COALESCE(c.company_name, ''),
  COALESCE(cs.name, ''),
  COALESCE(c.address, ''),
  COALESCE(c.website, '')
FROM customers c
LEFT JOIN customer_statuses cs ON cs.id = c.customer_status_id
WHERE c.company_id IN (%s) AND c.company_name LIKE ?
ORDER BY c.updated_at DESC
LIMIT 1`, strings.Join(placeholders, ","))
	return query, args
}

func (r *Repository) ProfileByName(ctx context.Context, coverage []int64, name string) (Profile, error) {
	if len(coverage) == 0 {
		return Profile{}, nil
	}
	query, args := buildProfileByNameQuery(coverage, name)

	var profile Profile
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&profile.Company, &profile.Status, &profile.Address, &profile.Website)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Profile{}, nil
		}
		return Profile{}, fmt.Errorf("customers: profile by name: %w", err)
	}
	profile.Found = true
	return profile, nil
}
