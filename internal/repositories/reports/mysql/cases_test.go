package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
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

func TestBuildCasesByProductQueryMatchesProductNode(t *testing.T) {
	query, args := buildCasesByProductQuery([]int64{3}, "Bella Bot", 20)
	if !strings.Contains(query, "JOIN nodes n ON n.id = r.product_node_id") {
		t.Fatalf("query = %q, want join on reports.product_node_id", query)
	}
	if strings.Contains(query, "report_node") {
		t.Fatalf("query = %q, report_node is the problem-type link, not the product", query)
	}
	if args[len(args)-2] != "%Bella Bot%" {
		t.Fatalf("args = %v, want product LIKE pattern", args)
	}
}

func TestBuildSearchCasesQueryScopesAndFilters(t *testing.T) {
	query, args := buildSearchCasesQuery([]int64{3, 4}, "printer", "Router X", "waiting", 5)
	if !strings.Contains(query, "r.company_id IN (?,?)") {
		t.Fatalf("query = %q, want coverage scope", query)
	}
	if !strings.Contains(query, "REPLACE(r.title, ' ', '') LIKE ? OR REPLACE(r.problem_text, ' ', '') LIKE ?") {
		t.Fatalf("query = %q, want space-stripped keyword filter over title+problem_text", query)
	}
	if strings.Contains(query, "problem_detail") {
		t.Fatalf("query = %q, problem_detail holds raw HTML and base64 images, search must use problem_text", query)
	}
	if !strings.Contains(query, "EXISTS (SELECT 1 FROM nodes n WHERE n.id = r.product_node_id") {
		t.Fatalf("query = %q, want product EXISTS filter on reports.product_node_id", query)
	}
	if !strings.Contains(query, "AND s.code = ?") {
		t.Fatalf("query = %q, want status filter", query)
	}
	// 2 coverage + 2 keyword + 1 product + 1 status + 1 limit
	if len(args) != 7 {
		t.Fatalf("args = %v, want 7", args)
	}
	if args[len(args)-1] != 5 {
		t.Fatalf("last arg = %v, want limit 5", args[len(args)-1])
	}
}

func TestBuildSearchCasesQueryStripsSpacesFromThaiQuery(t *testing.T) {
	_, args := buildSearchCasesQuery([]int64{3}, "หุ่นยนต์ ไม่ ทำงาน", "", "", 5)
	if args[1] != "%หุ่นยนต์ไม่ทำงาน%" {
		t.Fatalf("args[1] = %v, want spaces stripped so it matches unspaced Thai stored text", args[1])
	}
	if args[2] != "%หุ่นยนต์ไม่ทำงาน%" {
		t.Fatalf("args[2] = %v, want same stripped pattern for problem_text", args[2])
	}
}

func TestBuildCasesByIDsQueryScopesAndJoinsSymptom(t *testing.T) {
	query, args := buildCasesByIDsQuery([]int64{3, 4}, []int64{11, 12, 13})
	if !strings.Contains(query, "r.company_id IN (?,?)") {
		t.Fatalf("query = %q, want coverage scope", query)
	}
	if !strings.Contains(query, "r.id IN (?,?,?)") {
		t.Fatalf("query = %q, want id set filter", query)
	}
	if !strings.Contains(query, "LEFT JOIN report_classification rc ON rc.report_id = r.id AND rc.company_id = r.company_id") {
		t.Fatalf("query = %q, want classification join scoped by company", query)
	}
	if len(args) != 5 {
		t.Fatalf("args = %v, want 2 coverage + 3 ids", args)
	}
}

func TestBuildCasesByIDsQueryEmptyInputs(t *testing.T) {
	if q, _ := buildCasesByIDsQuery(nil, []int64{1}); q != "" {
		t.Fatalf("query = %q, want empty when coverage is empty", q)
	}
	if q, _ := buildCasesByIDsQuery([]int64{3}, nil); q != "" {
		t.Fatalf("query = %q, want empty when id set is empty", q)
	}
}

func TestBuildSearchCasesQueryOmitsAbsentFilters(t *testing.T) {
	query, args := buildSearchCasesQuery([]int64{3}, "", "", "", 0)
	if strings.Contains(query, "LIKE") || strings.Contains(query, "EXISTS") || strings.Contains(query, "s.code = ?") {
		t.Fatalf("query = %q, want no optional filters", query)
	}
	// 1 coverage + default limit
	if len(args) != 2 {
		t.Fatalf("args = %v, want 2 (coverage + default limit)", args)
	}
	if args[1] != 5 {
		t.Fatalf("default limit = %v, want 5", args[1])
	}
}

func TestSearchCasesEmptyCoverageReturnsEmpty(t *testing.T) {
	repo := New(&fakeCasesQuerier{})
	rows, err := repo.SearchCases(context.Background(), nil, "x", "", "", 5)
	if err != nil {
		t.Fatalf("SearchCases() error = %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %v, want empty for empty coverage", rows)
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

func TestBuildCaseByIDQueryScopesToCoverageAndExcludesCoded(t *testing.T) {
	query, args := buildCaseByIDQuery([]int64{3, 4}, 45)
	if !strings.Contains(query, "company_id IN (?,?)") {
		t.Fatalf("query = %q, want company_id IN (?,?)", query)
	}
	if !strings.Contains(query, "r.code IS NULL") {
		t.Fatalf("query = %q, want r.code IS NULL", query)
	}
	if len(args) != 3 {
		t.Fatalf("args = %v, want 3 args (id + 2 coverage)", args)
	}
	if args[0] != int64(45) {
		t.Fatalf("args[0] = %v, want id 45", args[0])
	}
	if args[1] != int64(3) || args[2] != int64(4) {
		t.Fatalf("args[1:] = %v, want [3 4]", args[1:])
	}
}

func TestCaseByIDEmptyCoverageReturnsErrorNoQuery(t *testing.T) {
	q := &fakeCasesQuerier{}
	repo := New(q)

	_, err := repo.CaseByID(context.Background(), nil, 45)
	if err == nil {
		t.Fatal("err = nil, want error for empty coverage")
	}
}

func TestCaseByIDNonPositiveIDReturnsError(t *testing.T) {
	q := &fakeCasesQuerier{}
	repo := New(q)

	_, err := repo.CaseByID(context.Background(), []int64{3}, 0)
	if err == nil {
		t.Fatal("err = nil, want error for non-positive id")
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

func TestBuildCaseByCodeQueryScopesToCoverageAndSelectsBackfillColumns(t *testing.T) {
	query, args := buildCaseByCodeQuery([]int64{3, 4}, "REP-4106")
	if !strings.Contains(query, "r.company_id IN (?,?)") {
		t.Fatalf("query = %q, want company_id IN (?,?)", query)
	}
	if !strings.Contains(query, "cu.id, si.id") {
		t.Fatalf("query = %q, want validated cu.id/si.id selected for backfill", query)
	}
	if !strings.Contains(query, "LEFT JOIN customers cu ON cu.id = r.customer_id AND cu.company_id = r.company_id") {
		t.Fatalf("query = %q, want customer existence+scope guard", query)
	}
	if !strings.Contains(query, "LEFT JOIN sites si ON si.id = r.site_id AND si.company_id = r.company_id") {
		t.Fatalf("query = %q, want site existence+scope guard", query)
	}
	if !strings.Contains(query, "r.code = ?") {
		t.Fatalf("query = %q, want code filter", query)
	}
	if len(args) != 3 || args[0] != "REP-4106" || args[1] != int64(3) || args[2] != int64(4) {
		t.Fatalf("args = %v, want [REP-4106 3 4]", args)
	}
}

func TestCaseByCodeReturnsCustomerAndSiteWhenPresent(t *testing.T) {
	db := newFakeScanDB(t, fakeScanRow{
		cols: []string{"code", "title", "status", "customer_id", "site_id"},
		vals: []driver.Value{"REP-4106", "Robot stuck", "open", int64(42), int64(7)},
	})
	repo := New(db)

	row, err := repo.CaseByCode(context.Background(), []int64{3}, "REP-4106")
	if err != nil {
		t.Fatalf("CaseByCode: %v", err)
	}
	if !row.CustomerID.Valid || row.CustomerID.Int64 != 42 {
		t.Fatalf("CustomerID = %+v, want valid 42", row.CustomerID)
	}
	if !row.SiteID.Valid || row.SiteID.Int64 != 7 {
		t.Fatalf("SiteID = %+v, want valid 7", row.SiteID)
	}
}

func TestCaseByCodeReturnsNullCustomerAndSiteWhenAbsent(t *testing.T) {
	db := newFakeScanDB(t, fakeScanRow{
		cols: []string{"code", "title", "status", "customer_id", "site_id"},
		vals: []driver.Value{"REP-4200", "No customer yet", "open", nil, nil},
	})
	repo := New(db)

	row, err := repo.CaseByCode(context.Background(), []int64{3}, "REP-4200")
	if err != nil {
		t.Fatalf("CaseByCode: %v", err)
	}
	if row.CustomerID.Valid {
		t.Fatalf("CustomerID = %+v, want null", row.CustomerID)
	}
	if row.SiteID.Valid {
		t.Fatalf("SiteID = %+v, want null", row.SiteID)
	}
}

type fakeScanRow struct {
	cols []string
	vals []driver.Value
	err  error
}

var (
	fakeScanMu         sync.Mutex
	fakeScanRegistry   = map[string]fakeScanRow{}
	fakeScanDriverOnce sync.Once
)

type fakeScanDriver struct{}

func (fakeScanDriver) Open(name string) (driver.Conn, error) {
	fakeScanMu.Lock()
	row, ok := fakeScanRegistry[name]
	fakeScanMu.Unlock()
	if !ok {
		return nil, errors.New("fakeScanDriver: unknown dsn")
	}
	return &fakeScanConn{row: row}, nil
}

type fakeScanConn struct {
	row fakeScanRow
}

func (c *fakeScanConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("fakeScanConn: Prepare unsupported")
}

func (c *fakeScanConn) Close() error { return nil }

func (c *fakeScanConn) Begin() (driver.Tx, error) {
	return nil, errors.New("fakeScanConn: Begin unsupported")
}

func (c *fakeScanConn) QueryContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	if c.row.err != nil {
		return nil, c.row.err
	}
	return &fakeScanRows{cols: c.row.cols, vals: c.row.vals}, nil
}

type fakeScanRows struct {
	cols []string
	vals []driver.Value
	done bool
}

func (r *fakeScanRows) Columns() []string { return r.cols }
func (r *fakeScanRows) Close() error      { return nil }

func (r *fakeScanRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(dest, r.vals)
	r.done = true
	return nil
}

func newFakeScanDB(t *testing.T, row fakeScanRow) *sql.DB {
	t.Helper()
	fakeScanDriverOnce.Do(func() {
		sql.Register("fakecasesscan", fakeScanDriver{})
	})
	dsn := t.Name()
	fakeScanMu.Lock()
	fakeScanRegistry[dsn] = row
	fakeScanMu.Unlock()
	db, err := sql.Open("fakecasesscan", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
