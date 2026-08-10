package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func buildSearchCasesQuery(coverage []int64, query, product, status string, limit int) (string, []any) {
	if len(coverage) == 0 {
		return "", nil
	}
	if limit <= 0 {
		limit = 5
	}
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+4)
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}

	var filters strings.Builder
	if query != "" {
		stripped := "%" + strings.ReplaceAll(query, " ", "") + "%"
		filters.WriteString(" AND (REPLACE(r.title, ' ', '') LIKE ? OR REPLACE(r.problem_text, ' ', '') LIKE ?)")
		args = append(args, stripped, stripped)
	}
	if product != "" {
		filters.WriteString(" AND EXISTS (SELECT 1 FROM nodes n WHERE n.id = r.product_node_id AND n.title LIKE ?)")
		args = append(args, "%"+product+"%")
	}
	if status != "" {
		filters.WriteString(" AND s.code = ?")
		args = append(args, status)
	}
	args = append(args, limit)

	q := fmt.Sprintf(`
SELECT r.id, COALESCE(r.code, ''), COALESCE(r.title, ''), COALESCE(s.code, '')
FROM reports r
LEFT JOIN report_statuses s ON s.id = r.status_id
WHERE r.company_id IN (%s)%s
ORDER BY r.created_at DESC
LIMIT ?`, strings.Join(placeholders, ","), filters.String())

	return q, args
}

func (r *Repository) SearchCases(ctx context.Context, coverage []int64, query, product, status string, limit int) ([]SearchRow, error) {
	if len(coverage) == 0 {
		return []SearchRow{}, nil
	}
	q, args := buildSearchCasesQuery(coverage, query, product, status, limit)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("reports: search cases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]SearchRow, 0)
	for rows.Next() {
		var row SearchRow
		var code, title, statusCode sql.NullString
		if err := rows.Scan(&row.ID, &code, &title, &statusCode); err != nil {
			return nil, fmt.Errorf("reports: search cases scan: %w", err)
		}
		row.Code = code.String
		row.Title = title.String
		row.Status = statusCode.String
		out = append(out, row)
	}
	return out, rows.Err()
}
