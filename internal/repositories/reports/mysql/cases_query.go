package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func buildLatestCasesQuery(coverage []int64, status string, scope string, limit int) (string, []any) {
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

	codeColumn := "r.code"
	scopeFilter := " AND r.code IS NOT NULL"
	switch scope {
	case "draft":
		codeColumn = "COALESCE(r.code, CONCAT('draft:', r.id))"
		scopeFilter = " AND r.code IS NULL"
	case "all":
		codeColumn = "COALESCE(r.code, CONCAT('draft:', r.id))"
		scopeFilter = ""
	}

	args = append(args, limit)

	query := fmt.Sprintf(`
SELECT %s, r.title, COALESCE(s.code, '')
FROM reports r
LEFT JOIN report_statuses s ON s.id = r.status_id
WHERE r.company_id IN (%s)%s%s
ORDER BY r.created_at DESC
LIMIT ?`, codeColumn, strings.Join(placeholders, ","), scopeFilter, statusFilter)

	return query, args
}

func (r *Repository) LatestCases(ctx context.Context, coverage []int64, status string, scope string, limit int) ([]CaseRow, error) {
	if len(coverage) == 0 {
		return []CaseRow{}, nil
	}
	query, args := buildLatestCasesQuery(coverage, status, scope, limit)
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
SELECT r.code, r.title, COALESCE(s.code, ''), cu.id, si.id
FROM reports r
LEFT JOIN report_statuses s ON s.id = r.status_id
LEFT JOIN customers cu ON cu.id = r.customer_id AND cu.company_id = r.company_id
LEFT JOIN sites si ON si.id = r.site_id AND si.company_id = r.company_id
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
	var customerID, siteID sql.NullInt64
	scanErr := traceScalarQuery(ctx, scalarTrace{
		stage:       "repository.reports.case_by_code",
		label:       "query case status from MySQL",
		method:      "CaseByCode",
		failedMsg:   "The parameterized case lookup failed.",
		notFoundMsg: "No case matched the supplied code inside the permitted company scope.",
		argCount:    len(args),
	}, func() error {
		return r.db.QueryRowContext(ctx, query, args...).Scan(&codeVal, &title, &statusCode, &customerID, &siteID)
	})
	if scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return CaseRow{}, fmt.Errorf("reports: case %q not found", code)
		}
		return CaseRow{}, fmt.Errorf("reports: case by code: %w", scanErr)
	}
	return CaseRow{Code: codeVal.String, Title: title.String, Status: statusCode.String, CustomerID: customerID, SiteID: siteID}, nil
}

func buildCaseByIDQuery(coverage []int64, id int64) (string, []any) {
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+1)
	args = append(args, id)
	for i, cid := range coverage {
		placeholders[i] = "?"
		args = append(args, cid)
	}
	query := fmt.Sprintf(`
SELECT r.code, r.title, COALESCE(s.code, '')
FROM reports r
LEFT JOIN report_statuses s ON s.id = r.status_id
WHERE r.id = ? AND r.code IS NULL AND r.company_id IN (%s)
LIMIT 1`, strings.Join(placeholders, ","))
	return query, args
}

func (r *Repository) CaseByID(ctx context.Context, coverage []int64, id int64) (CaseRow, error) {
	if len(coverage) == 0 {
		return CaseRow{}, errors.New("reports: coverage must not be empty")
	}
	if id <= 0 {
		return CaseRow{}, errors.New("reports: id must be positive")
	}
	query, args := buildCaseByIDQuery(coverage, id)

	var codeVal, title, statusCode sql.NullString
	scanErr := traceScalarQuery(ctx, scalarTrace{
		stage:       "repository.reports.case_by_id",
		label:       "query draft report from MySQL",
		method:      "CaseByID",
		failedMsg:   "The parameterized draft lookup failed.",
		notFoundMsg: "No code-less report matched the supplied id inside the permitted company scope.",
		argCount:    len(args),
	}, func() error {
		return r.db.QueryRowContext(ctx, query, args...).Scan(&codeVal, &title, &statusCode)
	})
	if scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return CaseRow{}, fmt.Errorf("reports: draft %d not found", id)
		}
		return CaseRow{}, fmt.Errorf("reports: case by id: %w", scanErr)
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
JOIN nodes n ON n.id = r.product_node_id
LEFT JOIN report_statuses s ON s.id = r.status_id
WHERE r.company_id IN (%s) AND r.code IS NOT NULL AND n.title LIKE ?
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
