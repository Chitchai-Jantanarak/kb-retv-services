package mysql

import (
	"context"
	"database/sql"
	"fmt"
)

func (r *Repository) queryCaseRows(
	ctx context.Context,
	query string,
	args []any,
	queryError string,
	scanError string,
) ([]CaseRow, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", queryError, err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]CaseRow, 0)
	for rows.Next() {
		var code, title, statusCode sql.NullString
		if err := rows.Scan(&code, &title, &statusCode); err != nil {
			return nil, fmt.Errorf("%s: %w", scanError, err)
		}
		out = append(out, CaseRow{Code: code.String, Title: title.String, Status: statusCode.String})
	}
	return out, rows.Err()
}
