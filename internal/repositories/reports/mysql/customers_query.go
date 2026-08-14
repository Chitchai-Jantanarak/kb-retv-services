package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// CustomerByEmailHash resolves a customer id from a sender-email HMAC digest,
// scoped to the coverage companies. It returns an invalid NullInt64 (not an
// error) when nothing matches, so the caller can treat "no match" and "no
// customer" the same way. The company_id filter is the existence + tenant
// guard: a hash that belongs to another company never resolves.
func (r *Repository) CustomerByEmailHash(ctx context.Context, coverage []int64, hash string) (sql.NullInt64, error) {
	if strings.TrimSpace(hash) == "" || len(coverage) == 0 {
		return sql.NullInt64{}, nil
	}

	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+1)
	args = append(args, hash)
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := fmt.Sprintf(
		`SELECT id FROM customers WHERE email_hash = ? AND company_id IN (%s) LIMIT 1`,
		strings.Join(placeholders, ","),
	)

	var id sql.NullInt64
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullInt64{}, nil
	}
	if err != nil {
		return sql.NullInt64{}, fmt.Errorf("customers: lookup by email hash: %w", err)
	}
	return id, nil
}
