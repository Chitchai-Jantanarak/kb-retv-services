package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func buildOpenCasesByEmployeeQuery(coverage []int64, name string, limit int) (string, []any) {
	if limit <= 0 {
		limit = 20
	}
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+2)
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, "%"+name+"%", limit)
	query := fmt.Sprintf(`
SELECT r.code, r.title, COALESCE(s.code, '')
FROM reports r
JOIN report_employee re ON re.report_id = r.id
JOIN employees e ON e.id = re.employee_id
LEFT JOIN report_statuses s ON s.id = r.status_id
WHERE r.company_id IN (%s)
  AND e.name LIKE ?
  AND (s.is_closed = 0 OR s.is_closed IS NULL)
ORDER BY r.created_at DESC
LIMIT ?`, strings.Join(placeholders, ","))
	return query, args
}

func (r *Repository) OpenCasesByEmployee(ctx context.Context, coverage []int64, name string, limit int) ([]CaseRow, error) {
	if len(coverage) == 0 {
		return []CaseRow{}, nil
	}
	query, args := buildOpenCasesByEmployeeQuery(coverage, name, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("employees: open cases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]CaseRow, 0)
	for rows.Next() {
		var code, title, statusCode sql.NullString
		if err := rows.Scan(&code, &title, &statusCode); err != nil {
			return nil, fmt.Errorf("employees: open cases scan: %w", err)
		}
		out = append(out, CaseRow{Code: code.String, Title: title.String, Status: statusCode.String})
	}
	return out, rows.Err()
}

func buildWorkloadQuery(coverage []int64, limit int) (string, []any) {
	if limit <= 0 {
		limit = 20
	}
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+1)
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, limit)
	query := fmt.Sprintf(`
SELECT e.name, COUNT(DISTINCT r.id), MIN(r.created_at)
FROM report_employee re
JOIN reports r ON r.id = re.report_id
JOIN employees e ON e.id = re.employee_id
LEFT JOIN report_statuses s ON s.id = r.status_id
WHERE r.company_id IN (%s)
  AND (s.is_closed = 0 OR s.is_closed IS NULL)
GROUP BY e.id, e.name
ORDER BY COUNT(DISTINCT r.id) DESC
LIMIT ?`, strings.Join(placeholders, ","))
	return query, args
}

func (r *Repository) Workload(ctx context.Context, coverage []int64, limit int) ([]WorkloadRow, error) {
	if len(coverage) == 0 {
		return []WorkloadRow{}, nil
	}
	query, args := buildWorkloadQuery(coverage, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("employees: workload: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]WorkloadRow, 0)
	for rows.Next() {
		var name sql.NullString
		var open sql.NullInt64
		var oldest sql.NullTime
		if err := rows.Scan(&name, &open, &oldest); err != nil {
			return nil, fmt.Errorf("employees: workload scan: %w", err)
		}
		row := WorkloadRow{Name: name.String, Open: open.Int64}
		if oldest.Valid {
			row.Oldest = oldest.Time.Format("2006-01-02")
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
