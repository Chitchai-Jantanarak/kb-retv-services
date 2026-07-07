package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func buildSelectPromotableConversationQuery(coverage []int64, conversationID int64) (string, []any) {
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+1)
	args = append(args, conversationID)
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(`
SELECT c.company_id, c.customer_id, COALESCE(c.subject, ''),
  COALESCE((SELECT m.body FROM messages m WHERE m.conversation_id = c.id ORDER BY m.received_at DESC LIMIT 1), '')
FROM conversations c
WHERE c.id = ? AND c.company_id IN (%s) AND c.status = 'pending'`, strings.Join(placeholders, ","))
	return query, args
}

func buildDefaultOpenStatusIDQuery() string {
	return `
SELECT id FROM report_statuses
WHERE code IN ('open', 'new', 'pending')
ORDER BY sort_order ASC
LIMIT 1`
}

func buildInsertReportFromConversationQuery(companyID int64, customerID sql.NullInt64, title, detail string, statusID sql.NullInt64) (string, []any) {
	query := `
INSERT INTO reports (title, problem_detail, company_id, customer_id, status_id)
VALUES (?, ?, ?, ?, ?)`
	return query, []any{title, detail, companyID, customerID, statusID}
}

func buildSetReportCodeQuery(reportID int64, code string) (string, []any) {
	return `UPDATE reports SET code = ? WHERE id = ?`, []any{code, reportID}
}

func buildMarkConversationPromotedQuery(coverage []int64, conversationID, reportID int64) (string, []any) {
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage)+2)
	args = append(args, reportID, conversationID)
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(`
UPDATE conversations
SET status = 'promoted', report_id = ?
WHERE id = ? AND company_id IN (%s)`, strings.Join(placeholders, ","))
	return query, args
}

func (r *Repository) PromoteConversation(ctx context.Context, coverage []int64, conversationID, actorEmployeeID int64) (string, error) {
	if len(coverage) == 0 {
		return "", errors.New("reports: coverage must not be empty")
	}
	if conversationID <= 0 {
		return "", errors.New("reports: conversation_id must be positive")
	}
	if actorEmployeeID <= 0 {
		return "", errors.New("reports: actor_employee_id must be positive")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("reports: promote conversation: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	selectQuery, selectArgs := buildSelectPromotableConversationQuery(coverage, conversationID)
	var companyID int64
	var customerID sql.NullInt64
	var subject, body string
	if err := tx.QueryRowContext(ctx, selectQuery, selectArgs...).Scan(&companyID, &customerID, &subject, &body); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("reports: conversation %d not found or not pending", conversationID)
		}
		return "", fmt.Errorf("reports: promote conversation: select: %w", err)
	}

	var statusID sql.NullInt64
	if err := tx.QueryRowContext(ctx, buildDefaultOpenStatusIDQuery()).Scan(&statusID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("reports: promote conversation: resolve open status: %w", err)
	}

	insertQuery, insertArgs := buildInsertReportFromConversationQuery(companyID, customerID, subject, body, statusID)
	res, err := tx.ExecContext(ctx, insertQuery, insertArgs...)
	if err != nil {
		return "", fmt.Errorf("reports: promote conversation: insert report: %w", err)
	}
	reportID, err := res.LastInsertId()
	if err != nil {
		return "", fmt.Errorf("reports: promote conversation: report id: %w", err)
	}

	code := fmt.Sprintf("REP-%d", reportID)
	codeQuery, codeArgs := buildSetReportCodeQuery(reportID, code)
	if _, err := tx.ExecContext(ctx, codeQuery, codeArgs...); err != nil {
		return "", fmt.Errorf("reports: promote conversation: set code: %w", err)
	}

	markQuery, markArgs := buildMarkConversationPromotedQuery(coverage, conversationID, reportID)
	if _, err := tx.ExecContext(ctx, markQuery, markArgs...); err != nil {
		return "", fmt.Errorf("reports: promote conversation: mark promoted: %w", err)
	}

	assignQuery, assignArgs := buildAssignCaseQuery(coverage, reportID, actorEmployeeID)
	if _, err := tx.ExecContext(ctx, assignQuery, assignArgs...); err != nil {
		return "", fmt.Errorf("reports: promote conversation: assign employee: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("reports: promote conversation: commit: %w", err)
	}
	return code, nil
}
