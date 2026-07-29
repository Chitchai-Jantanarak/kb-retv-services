package mysql

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

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
