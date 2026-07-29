package mysql

import (
	"context"
	"errors"
	"fmt"
)

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
