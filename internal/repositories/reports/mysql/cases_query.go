package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/my/app/internal/shared/debugtrace"
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
	start := time.Now()
	scanErr := r.db.QueryRowContext(ctx, query, args...).Scan(&codeVal, &title, &statusCode)
	state := "completed"
	errorCode := ""
	safeError := ""
	rowCount := "1"
	if scanErr != nil {
		state = "failed"
		errorCode = "query_failed"
		safeError = "The parameterized case lookup failed."
		rowCount = "0"
		if errors.Is(scanErr, sql.ErrNoRows) {
			state = "not_found"
			errorCode = "not_found"
			safeError = "No case matched the supplied code inside the permitted company scope."
		}
	}
	debugtrace.Add(ctx, debugtrace.Event{
		Stage:      "repository.reports.case_by_code",
		State:      state,
		Label:      "query case status from MySQL",
		DurationMS: debugtrace.Since(start),
		ErrorCode:  errorCode,
		Error:      safeError,
		Context: map[string]string{
			"repository":      "internal/repositories/reports/mysql",
			"method":          "CaseByCode",
			"operation":       "SELECT",
			"tables":          "reports, report_statuses",
			"scope":           "company coverage",
			"parameterized":   "true",
			"parameter_count": strconv.Itoa(len(args)),
			"limit":           "1",
			"rows":            rowCount,
		},
	})
	if scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return CaseRow{}, fmt.Errorf("reports: case %q not found", code)
		}
		return CaseRow{}, fmt.Errorf("reports: case by code: %w", scanErr)
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
