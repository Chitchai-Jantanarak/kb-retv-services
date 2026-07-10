package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type CaseRow struct {
	Code   string
	Title  string
	Status string
}

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

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("reports: latest cases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]CaseRow, 0)
	for rows.Next() {
		var code, title, statusCode sql.NullString
		if err := rows.Scan(&code, &title, &statusCode); err != nil {
			return nil, fmt.Errorf("reports: latest cases scan: %w", err)
		}
		out = append(out, CaseRow{Code: code.String, Title: title.String, Status: statusCode.String})
	}
	return out, rows.Err()
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

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("reports: cases by product: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]CaseRow, 0)
	for rows.Next() {
		var code, title, statusCode sql.NullString
		if err := rows.Scan(&code, &title, &statusCode); err != nil {
			return nil, fmt.Errorf("reports: cases by product scan: %w", err)
		}
		out = append(out, CaseRow{Code: code.String, Title: title.String, Status: statusCode.String})
	}
	return out, rows.Err()
}

func buildIsResponsibleQuery(companyID, reportID, userID int64) (string, []any) {
	query := `
SELECT COUNT(*)
FROM report_employee re
JOIN reports r ON r.id = re.report_id
WHERE re.report_id = ? AND re.employee_id = ? AND r.company_id = ?`
	return query, []any{reportID, userID, companyID}
}

func (r *Repository) IsResponsible(ctx context.Context, companyID, reportID, userID int64) (bool, error) {
	if companyID <= 0 || reportID <= 0 || userID <= 0 {
		return false, errors.New("reports: company_id, report_id, and user_id must be positive")
	}
	query, args := buildIsResponsibleQuery(companyID, reportID, userID)

	var count int64
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return false, fmt.Errorf("reports: is responsible: %w", err)
	}
	return count > 0, nil
}

func buildUpdateCaseStatusQuery(coverage []int64, reportID int64, statusCode string) (string, []any) {
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+2)
	args = append(args, statusCode, reportID)
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(`
UPDATE reports
SET status_id = (SELECT id FROM report_statuses WHERE code = ?)
WHERE id = ? AND company_id IN (%s)`, strings.Join(placeholders, ","))
	return query, args
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

func (r *Repository) UpdateCaseStatus(ctx context.Context, coverage []int64, reportID int64, statusCode string) error {
	if len(coverage) == 0 {
		return errors.New("reports: coverage must not be empty")
	}
	if reportID <= 0 {
		return errors.New("reports: report_id must be positive")
	}
	if statusCode == "" {
		return errors.New("reports: status_code must not be empty")
	}
	query, args := buildUpdateCaseStatusQuery(coverage, reportID, statusCode)

	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("reports: update case status: %w", err)
	}
	return nil
}

func buildAssignCaseQuery(coverage []int64, reportID, employeeID int64) (string, []any) {
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+3)
	args = append(args, reportID, employeeID, reportID)
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(`
INSERT INTO report_employee (report_id, employee_id)
SELECT ?, ? FROM reports
WHERE id = ? AND company_id IN (%s)`, strings.Join(placeholders, ","))
	return query, args
}

func (r *Repository) AssignCase(ctx context.Context, coverage []int64, reportID, employeeID int64) error {
	if len(coverage) == 0 {
		return errors.New("reports: coverage must not be empty")
	}
	if reportID <= 0 {
		return errors.New("reports: report_id must be positive")
	}
	if employeeID <= 0 {
		return errors.New("reports: employee_id must be positive")
	}
	query, args := buildAssignCaseQuery(coverage, reportID, employeeID)

	if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("reports: assign case: %w", err)
	}
	return nil
}
