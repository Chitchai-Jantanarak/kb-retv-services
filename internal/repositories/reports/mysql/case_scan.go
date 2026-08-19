package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/my/app/internal/shared/debugtrace"
)

type scalarTrace struct {
	stage       string
	label       string
	method      string
	failedMsg   string
	notFoundMsg string
	argCount    int
}

func traceScalarQuery(ctx context.Context, t scalarTrace, scan func() error) error {
	start := time.Now()
	scanErr := scan()
	state := "completed"
	errorCode := ""
	safeError := ""
	rowCount := "1"
	if scanErr != nil {
		state = "failed"
		errorCode = "query_failed"
		safeError = t.failedMsg
		rowCount = "0"
		if errors.Is(scanErr, sql.ErrNoRows) {
			state = "not_found"
			errorCode = "not_found"
			safeError = t.notFoundMsg
		}
	}
	debugtrace.Add(ctx, debugtrace.Event{
		Stage:      t.stage,
		State:      state,
		Label:      t.label,
		DurationMS: debugtrace.Since(start),
		ErrorCode:  errorCode,
		Error:      safeError,
		Context: map[string]string{
			"repository":      "internal/repositories/reports/mysql",
			"method":          t.method,
			"operation":       "SELECT",
			"tables":          "reports, report_statuses",
			"scope":           "company coverage",
			"parameterized":   "true",
			"parameter_count": strconv.Itoa(t.argCount),
			"limit":           "1",
			"rows":            rowCount,
		},
	})
	return scanErr
}

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
