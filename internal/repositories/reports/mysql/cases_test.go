package mysql

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

type fakeCasesQuerier struct {
	queryCalled bool
	execCalled  bool
	execSQL     string
	execArgs    []any
}

func (f *fakeCasesQuerier) QueryContext(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	f.queryCalled = true
	return nil, errors.New("fakeCasesQuerier: query not supported")
}

func (f *fakeCasesQuerier) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }

func (f *fakeCasesQuerier) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	f.execCalled = true
	f.execSQL = query
	f.execArgs = args
	return fakeCasesResult{}, nil
}

func (f *fakeCasesQuerier) BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error) { return nil, nil }

type fakeCasesResult struct{}

func (fakeCasesResult) LastInsertId() (int64, error) { return 0, nil }
func (fakeCasesResult) RowsAffected() (int64, error) { return 1, nil }

func TestBuildLatestCasesQueryScopesToCoverage(t *testing.T) {
	query, args := buildLatestCasesQuery([]int64{3, 4}, "", 10)
	if !strings.Contains(query, "r.company_id IN (?,?)") {
		t.Fatalf("query = %q, want company_id IN (?,?)", query)
	}
	if len(args) < 2 || args[0] != int64(3) || args[1] != int64(4) {
		t.Fatalf("args = %v, want [3 4 ...]", args)
	}
}

func TestBuildLatestCasesQueryWithStatusFilter(t *testing.T) {
	query, args := buildLatestCasesQuery([]int64{3, 4}, "open", 10)
	if !strings.Contains(query, "AND s.code = ?") {
		t.Fatalf("query = %q, want status filter", query)
	}
	if len(args) != 4 {
		t.Fatalf("args = %v, want 4 args (2 coverage + status + limit)", args)
	}
	if args[2] != "open" {
		t.Fatalf("args[2] = %v, want status code", args[2])
	}
	if args[3] != 10 {
		t.Fatalf("args[3] = %v, want limit", args[3])
	}
}

func TestBuildLatestCasesQueryWithoutStatusOmitsFilter(t *testing.T) {
	query, args := buildLatestCasesQuery([]int64{3, 4}, "", 10)
	if strings.Contains(query, "s.code = ?") {
		t.Fatalf("query = %q, must not filter by status when empty", query)
	}
	if len(args) != 3 {
		t.Fatalf("args = %v, want 3 args (2 coverage + limit)", args)
	}
}

func TestLatestCasesEmptyCoverageRunsNoQuery(t *testing.T) {
	q := &fakeCasesQuerier{}
	repo := New(q)

	rows, err := repo.LatestCases(context.Background(), nil, "", 10)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %v, want empty slice", rows)
	}
	if q.queryCalled {
		t.Fatal("QueryContext must not be called when coverage is empty")
	}
}

func TestBuildIsResponsibleQueryBindsReportAndUser(t *testing.T) {
	query, args := buildIsResponsibleQuery(7, 101, 55)
	if !strings.Contains(query, "re.report_id = ?") || !strings.Contains(query, "re.employee_id = ?") {
		t.Fatalf("query = %q, want report_id/employee_id filters", query)
	}
	if !strings.Contains(query, "r.company_id = ?") {
		t.Fatalf("query = %q, want company_id scoping join", query)
	}
	if len(args) != 3 {
		t.Fatalf("args = %v, want 3 args", args)
	}
	if args[0] != int64(101) {
		t.Fatalf("args[0] = %v, want reportID 101", args[0])
	}
	if args[1] != int64(55) {
		t.Fatalf("args[1] = %v, want userID 55", args[1])
	}
	if args[2] != int64(7) {
		t.Fatalf("args[2] = %v, want companyID 7", args[2])
	}
}

func TestIsResponsibleRejectsNonPositiveIDs(t *testing.T) {
	repo := New(nil)
	cases := []struct {
		name      string
		companyID int64
		reportID  int64
		userID    int64
	}{
		{name: "zero_company", companyID: 0, reportID: 1, userID: 1},
		{name: "zero_report", companyID: 1, reportID: 0, userID: 1},
		{name: "zero_user", companyID: 1, reportID: 1, userID: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.IsResponsible(context.Background(), tc.companyID, tc.reportID, tc.userID)
			if err == nil {
				t.Fatal("err = nil, want validation error")
			}
		})
	}
}

func TestBuildCaseIDByCodeQueryScopesToCoverage(t *testing.T) {
	query, args := buildCaseIDByCodeQuery([]int64{3, 4}, "REP-100")
	if !strings.Contains(query, "company_id IN (?,?)") {
		t.Fatalf("query = %q, want company_id IN (?,?)", query)
	}
	if len(args) != 3 {
		t.Fatalf("args = %v, want 3 args (code + 2 coverage)", args)
	}
	if args[0] != "REP-100" {
		t.Fatalf("args[0] = %v, want code REP-100", args[0])
	}
	if args[1] != int64(3) || args[2] != int64(4) {
		t.Fatalf("args[1:] = %v, want [3 4]", args[1:])
	}
}

func TestCaseIDByCodeEmptyCoverageReturnsErrorNoQuery(t *testing.T) {
	q := &fakeCasesQuerier{}
	repo := New(q)

	_, err := repo.CaseIDByCode(context.Background(), nil, "REP-100")
	if err == nil {
		t.Fatal("err = nil, want error for empty coverage")
	}
}

func TestCaseIDByCodeEmptyCodeReturnsError(t *testing.T) {
	q := &fakeCasesQuerier{}
	repo := New(q)

	_, err := repo.CaseIDByCode(context.Background(), []int64{3}, "")
	if err == nil {
		t.Fatal("err = nil, want error for empty code")
	}
}

func TestUpdateCaseStatusEmptyCoverageReturnsErrorNoExec(t *testing.T) {
	q := &fakeCasesQuerier{}
	repo := New(q)

	err := repo.UpdateCaseStatus(context.Background(), nil, 1, "closed")
	if err == nil {
		t.Fatal("err = nil, want error for empty coverage")
	}
	if q.execCalled {
		t.Fatal("ExecContext must not be called when coverage is empty")
	}
}

func TestUpdateCaseStatusScopesToCoverage(t *testing.T) {
	q := &fakeCasesQuerier{}
	repo := New(q)

	if err := repo.UpdateCaseStatus(context.Background(), []int64{3, 4}, 1, "closed"); err != nil {
		t.Fatalf("UpdateCaseStatus: %v", err)
	}
	if !q.execCalled {
		t.Fatal("ExecContext should be called when coverage is not empty")
	}
	if !strings.Contains(q.execSQL, "company_id IN (?,?)") {
		t.Fatalf("execSQL = %q, want company_id IN (?,?)", q.execSQL)
	}
	found := false
	for _, a := range q.execArgs {
		if a == "closed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("status code not bound in args: %v", q.execArgs)
	}
}

func TestBuildAssignCaseQueryScopesToCoverage(t *testing.T) {
	query, args := buildAssignCaseQuery([]int64{3, 4}, 100, 55)
	if !strings.Contains(query, "company_id IN (?,?)") {
		t.Fatalf("query = %q, want company_id IN (?,?)", query)
	}
	if len(args) != 5 {
		t.Fatalf("args = %v, want 5 args (report_id + employee_id + report_id + 2 coverage)", args)
	}
	if args[0] != int64(100) || args[1] != int64(55) || args[2] != int64(100) {
		t.Fatalf("args = %v, want [100 55 100 3 4]", args)
	}
	if args[3] != int64(3) || args[4] != int64(4) {
		t.Fatalf("args = %v, want coverage [3 4] appended", args)
	}
}

func TestAssignCaseEmptyCoverageReturnsErrorNoExec(t *testing.T) {
	q := &fakeCasesQuerier{}
	repo := New(q)

	err := repo.AssignCase(context.Background(), nil, 100, 55)
	if err == nil {
		t.Fatal("err = nil, want error for empty coverage")
	}
	if q.execCalled {
		t.Fatal("ExecContext must not be called when coverage is empty")
	}
}

func TestAssignCaseRejectsNonPositiveIDs(t *testing.T) {
	q := &fakeCasesQuerier{}
	repo := New(q)

	if err := repo.AssignCase(context.Background(), []int64{3}, 0, 55); err == nil {
		t.Fatal("err = nil, want error for non-positive report_id")
	}
	if err := repo.AssignCase(context.Background(), []int64{3}, 100, 0); err == nil {
		t.Fatal("err = nil, want error for non-positive employee_id")
	}
	if q.execCalled {
		t.Fatal("ExecContext must not be called when ids are invalid")
	}
}

func TestAssignCaseScopesToCoverage(t *testing.T) {
	q := &fakeCasesQuerier{}
	repo := New(q)

	if err := repo.AssignCase(context.Background(), []int64{3, 4}, 100, 55); err != nil {
		t.Fatalf("AssignCase: %v", err)
	}
	if !q.execCalled {
		t.Fatal("ExecContext should be called when coverage is not empty")
	}
	if !strings.Contains(q.execSQL, "company_id IN (?,?)") {
		t.Fatalf("execSQL = %q, want company_id IN (?,?)", q.execSQL)
	}
	if !strings.Contains(q.execSQL, "INSERT INTO report_employee") {
		t.Fatalf("execSQL = %q, want INSERT INTO report_employee", q.execSQL)
	}
}
