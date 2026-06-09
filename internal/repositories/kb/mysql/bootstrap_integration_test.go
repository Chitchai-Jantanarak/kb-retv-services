package mysql

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func TestReportSourceReadsV4DetailsIntegration(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN is not set")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var reportID, companyID int64
	var wantProblemDetail, wantFixDetail string
	err = db.QueryRowContext(ctx, `
SELECT id, company_id, problem_detail, fix_detail
FROM reports
WHERE COALESCE(problem_detail, '') <> ''
  AND COALESCE(fix_detail, '') <> ''
ORDER BY id ASC
LIMIT 1`).Scan(&reportID, &companyID, &wantProblemDetail, &wantFixDetail)
	if err == sql.ErrNoRows {
		t.Skip("no V4 report with both problem and fix details is available")
	}
	if err != nil {
		t.Fatalf("select sample V4 report: %v", err)
	}

	reports, err := NewReportSource(db).Reports(ctx, companyID, 0)
	if err != nil {
		t.Fatalf("Reports() error = %v", err)
	}

	for _, report := range reports {
		if report.ID != reportID {
			continue
		}
		if report.ProblemDetail != wantProblemDetail {
			t.Fatalf("ProblemDetail = %q, want %q", report.ProblemDetail, wantProblemDetail)
		}
		if report.FixProblem != wantFixDetail {
			t.Fatalf("FixProblem = %q, want %q", report.FixProblem, wantFixDetail)
		}
		return
	}

	t.Fatalf("Reports() did not return V4 report %d", reportID)
}
