package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func buildLatestCasesQuery(coverage []int64, status string, limit int) (string, []any) {
	if len(coverage) == 0 {
		return "", nil
	}
	if limit <= 0 {
		limit = 20
	}
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+2)
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}
	statusFilter := ""
	if status != "" {
		statusFilter = " AND s.code = ?"
		args = append(args, status)
	}
	args = append(args, limit)

	query := fmt.Sprintf(`
SELECT r.code, r.title, COALESCE(s.code, '')
FROM reports r
LEFT JOIN report_statuses s ON s.id = r.status_id
WHERE r.company_id IN (%s)%s
ORDER BY r.created_at DESC
LIMIT ?`, strings.Join(placeholders, ","), statusFilter)

	return query, args
}

func (r *Repository) LatestCases(ctx context.Context, coverage []int64, status string, limit int) ([]CaseRow, error) {
	if len(coverage) == 0 {
		return []CaseRow{}, nil
	}
	query, args := buildLatestCasesQuery(coverage, status, limit)
	return r.queryCaseRows(
		ctx,
		query,
		args,
		"reports: latest cases",
		"reports: latest cases scan",
	)
}

func buildCaseByCodeQuery(coverage []int64, code string) (string, []any) {
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+1)
	args = append(args, code)
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(`
SELECT r.code, r.title, COALESCE(s.code, '')
FROM reports r
LEFT JOIN report_statuses s ON s.id = r.status_id
WHERE r.code = ? AND r.company_id IN (%s)
LIMIT 1`, strings.Join(placeholders, ","))
	return query, args
}

func (r *Repository) CaseByCode(ctx context.Context, coverage []int64, code string) (CaseRow, error) {
	if len(coverage) == 0 {
		return CaseRow{}, errors.New("reports: coverage must not be empty")
	}
	if code == "" {
		return CaseRow{}, errors.New("reports: code must not be empty")
	}
	query, args := buildCaseByCodeQuery(coverage, code)

	var codeVal, title, statusCode sql.NullString
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&codeVal, &title, &statusCode); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CaseRow{}, fmt.Errorf("reports: case %q not found", code)
		}
		return CaseRow{}, fmt.Errorf("reports: case by code: %w", err)
	}
	return CaseRow{Code: codeVal.String, Title: title.String, Status: statusCode.String}, nil
}

func buildCasesByProductQuery(coverage []int64, product string, limit int) (string, []any) {
	if limit <= 0 {
		limit = 20
	}
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+2)
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, "%"+product+"%", limit)
	query := fmt.Sprintf(`
SELECT r.code, r.title, COALESCE(s.code, '')
FROM reports r
JOIN report_node rn ON rn.report_id = r.id
JOIN nodes n ON n.id = rn.node_id
LEFT JOIN report_statuses s ON s.id = r.status_id
WHERE r.company_id IN (%s) AND n.title LIKE ?
ORDER BY r.created_at DESC
LIMIT ?`, strings.Join(placeholders, ","))
	return query, args
}

func (r *Repository) CasesByProduct(ctx context.Context, coverage []int64, product string, limit int) ([]CaseRow, error) {
	if len(coverage) == 0 {
		return []CaseRow{}, nil
	}
	query, args := buildCasesByProductQuery(coverage, product, limit)
	return r.queryCaseRows(
		ctx,
		query,
		args,
		"reports: cases by product",
		"reports: cases by product scan",
	)
}

func buildCaseIDByCodeQuery(coverage []int64, code string) (string, []any) {
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+1)
	args = append(args, code)
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(`
SELECT id FROM reports
WHERE code = ? AND company_id IN (%s)
LIMIT 1`, strings.Join(placeholders, ","))
	return query, args
}

func (r *Repository) CaseIDByCode(ctx context.Context, coverage []int64, code string) (int64, error) {
	if len(coverage) == 0 {
		return 0, errors.New("reports: coverage must not be empty")
	}
	if code == "" {
		return 0, errors.New("reports: code must not be empty")
	}
	query, args := buildCaseIDByCodeQuery(coverage, code)

	var id int64
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("reports: case %q not found", code)
		}
		return 0, fmt.Errorf("reports: case id by code: %w", err)
	}
	return id, nil
}
