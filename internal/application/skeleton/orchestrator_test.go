package skeleton

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/my/app/internal/application/tools"
)

type fakeSelector struct{ sel tools.Selection }

func (f fakeSelector) Select(_ context.Context, _ string, _ []string) (tools.Selection, error) {
	return f.sel, nil
}

type fakeHandler struct {
	got   Query
	calls int
}

func (f *fakeHandler) Run(_ context.Context, q Query) ([]Row, error) {
	f.got = q
	f.calls++
	return []Row{{"code": "R1"}, {"code": "R2"}}, nil
}

type failHandler struct{ calls int }

func (f *failHandler) Run(_ context.Context, _ Query) ([]Row, error) {
	f.calls++
	return nil, fmt.Errorf("handler boom")
}

type failAuditor struct{ calls int }

func (f *failAuditor) Log(_ context.Context, _ string, _ Actor, _ string) error {
	f.calls++
	return fmt.Errorf("audit boom")
}

type fakeAuditor struct {
	tag    string
	toolID string
	calls  int
}

func (f *fakeAuditor) Log(_ context.Context, tag string, _ Actor, toolID string) error {
	f.tag = tag
	f.toolID = toolID
	f.calls++
	return nil
}

func findTool() tools.Tool {
	return tools.Tool{
		ID:      "f1_find_cases",
		Handler: "reports.find",
		Audit:   "ai.find_cases",
		Compose: tools.Compose{Headline: "{n} cases match", Row: "{code}", Cite: "code"},
	}
}

func writeTool() tools.Tool {
	return tools.Tool{
		ID:      "f5_close_case",
		Kind:    "write",
		Handler: "reports.close",
		Audit:   "ai.close_case",
		Compose: tools.Compose{Headline: "{n} cases closed", Row: "{code}", Cite: "code"},
	}
}

type productHandler struct{ rows []Row }

func (p productHandler) Run(_ context.Context, _ Query) ([]Row, error) { return p.rows, nil }

func productTool() tools.Tool {
	return tools.Tool{
		ID:      "f5_product_cases",
		Handler: "reports.byProduct",
		Audit:   "ai.product_cases",
		Compose: tools.Compose{Headline: "{n} cases for {product}", Row: "{code}|{title}|{status}|{assignee}", Cite: "code"},
	}
}

func composeFor(t *testing.T, tool tools.Tool, rows []Row) Response {
	t.Helper()
	sel := fakeSelector{sel: tools.Selection{ToolID: tool.ID, Matched: true}}
	o := New(sel, []tools.Tool{tool}, map[string]Handler{tool.Handler: productHandler{rows: rows}}, &fakeAuditor{})
	resp, err := o.Handle(context.Background(), Actor{CompanyID: 3, Coverage: []int64{3}}, "cases for wifi")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	return resp
}

func pendingSetup(t *testing.T, h Handler, a Auditor) (*Orchestrator, *PendingStore) {
	t.Helper()
	store := NewPendingStore()
	sel := fakeSelector{sel: tools.Selection{
		ToolID:  "f5_close_case",
		Matched: true,
		Params:  map[string]string{"code": "REP-7"},
	}}
	tool := writeTool()
	tool.Compose.Headline = "case {code} closed"
	tool.RBAC.RequiresPermission = "report.update"
	o := New(sel, []tools.Tool{tool}, map[string]Handler{"reports.close": h}, a, WithPendingStore(store))
	return o, store
}

func TestWriteToolProposesInsteadOfExecuting(t *testing.T) {
	h := &fakeHandler{}
	a := &fakeAuditor{}
	o, _ := pendingSetup(t, h, a)

	resp, err := o.Handle(context.Background(), Actor{CompanyID: 3, UserID: 9, Perms: []string{"report.update"}, Coverage: []int64{3}}, "close REP-7")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if h.calls != 0 {
		t.Fatalf("handler ran %d times on propose, want 0", h.calls)
	}
	if resp.Pending == nil || resp.Pending.ID == "" {
		t.Fatalf("no pending ref: %+v", resp)
	}
	if resp.Pending.Summary != "case REP-7 closed" {
		t.Fatalf("summary = %q", resp.Pending.Summary)
	}
	if a.tag != "ai.close_case.proposed" {
		t.Fatalf("audit tag = %q, want proposed", a.tag)
	}
}

func TestConfirmExecutesOnceOnly(t *testing.T) {
	h := &fakeHandler{}
	a := &fakeAuditor{}
	o, _ := pendingSetup(t, h, a)
	actor := Actor{CompanyID: 3, UserID: 9, Perms: []string{"report.update"}, Coverage: []int64{3}}

	resp, err := o.Handle(context.Background(), actor, "close REP-7")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	confirmed, err := o.Confirm(context.Background(), actor, resp.Pending.ID)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if h.calls != 1 || !confirmed.Called {
		t.Fatalf("calls = %d Called = %v, want 1/true", h.calls, confirmed.Called)
	}
	if a.tag != "ai.close_case" {
		t.Fatalf("audit tag = %q, want real tag on execution", a.tag)
	}

	if _, err := o.Confirm(context.Background(), actor, resp.Pending.ID); err == nil {
		t.Fatal("second confirm must fail")
	}
	if h.calls != 1 {
		t.Fatalf("handler ran again on replay: %d", h.calls)
	}
}

func TestConfirmRejectsWrongActor(t *testing.T) {
	h := &fakeHandler{}
	o, _ := pendingSetup(t, h, &fakeAuditor{})
	actor := Actor{CompanyID: 3, UserID: 9, Perms: []string{"report.update"}, Coverage: []int64{3}}

	resp, err := o.Handle(context.Background(), actor, "close REP-7")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	other := Actor{CompanyID: 3, UserID: 10, Perms: []string{"report.update"}, Coverage: []int64{3}}
	if _, err := o.Confirm(context.Background(), other, resp.Pending.ID); err == nil {
		t.Fatal("confirm by another user must fail")
	}
	if h.calls != 0 {
		t.Fatalf("handler ran for wrong actor: %d", h.calls)
	}
}

func TestConfirmReauthorizes(t *testing.T) {
	h := &fakeHandler{}
	o, _ := pendingSetup(t, h, &fakeAuditor{})
	actor := Actor{CompanyID: 3, UserID: 9, Perms: []string{"report.update"}, Coverage: []int64{3}}

	resp, err := o.Handle(context.Background(), actor, "close REP-7")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	stripped := Actor{CompanyID: 3, UserID: 9, Perms: nil, Coverage: []int64{3}}
	if _, err := o.Confirm(context.Background(), stripped, resp.Pending.ID); err == nil {
		t.Fatal("confirm without permission must fail")
	}
	if h.calls != 0 {
		t.Fatalf("handler ran without permission: %d", h.calls)
	}
}

func TestWriteParamsComeFromSelectionOnly(t *testing.T) {
	h := &fakeHandler{}
	sel := fakeSelector{sel: tools.Selection{
		ToolID:  "f5_close_case",
		Matched: true,
		Params:  map[string]string{"code": "REP-7"},
	}}
	o := New(sel, []tools.Tool{writeTool()}, map[string]Handler{"reports.close": h}, &fakeAuditor{})

	if _, err := o.Handle(context.Background(), Actor{CompanyID: 3, Coverage: []int64{3}}, "close REP-7 please"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !reflect.DeepEqual(h.got.Params, map[string]string{"code": "REP-7"}) {
		t.Fatalf("handler params = %v, want exactly the selection params", h.got.Params)
	}
}

func columnsTool() tools.Tool {
	return tools.Tool{
		ID:      "f1_find_cases",
		Handler: "reports.find",
		Audit:   "ai.find_cases",
		Compose: tools.Compose{
			Mode:     "summary",
			Headline: "{n} cases match",
			Columns: []tools.ToolColumn{
				{Key: "code", Type: "case_ref", Primary: true},
				{Key: "title", Type: "text"},
				{Key: "status", Type: "status"},
			},
		},
	}
}

func TestComposeBuildsToolTable(t *testing.T) {
	resp := composeFor(t, columnsTool(), []Row{
		{"code": "REP-1", "title": "wifi down", "status": "waiting"},
		{"code": "REP-2", "title": "printer", "status": "done"},
	})
	if resp.Table == nil {
		t.Fatal("Table is nil")
	}
	if resp.Table.ToolID != "f1_find_cases" {
		t.Fatalf("ToolID = %q", resp.Table.ToolID)
	}
	if len(resp.Table.Columns) != 3 || resp.Table.Columns[0].Key != "code" || !resp.Table.Columns[0].Primary {
		t.Fatalf("columns = %+v", resp.Table.Columns)
	}
	if len(resp.Table.Rows) != 2 || resp.Table.Rows[0]["code"] != "REP-1" {
		t.Fatalf("rows = %+v", resp.Table.Rows)
	}
}

func TestComposeToolTablePresentWithZeroRows(t *testing.T) {
	resp := composeFor(t, columnsTool(), nil)
	if resp.Table == nil || resp.Table.Rows == nil || len(resp.Table.Rows) != 0 {
		t.Fatalf("want empty non-nil rows, got %+v", resp.Table)
	}
}

func TestComposeFillsHeadlineFromFirstRow(t *testing.T) {
	resp := composeFor(t, productTool(), []Row{
		{"code": "REP-1", "title": "wifi down", "status": "waiting", "product": "wifi"},
	})
	if resp.Headline != "1 cases for wifi" {
		t.Fatalf("headline = %q, want %q", resp.Headline, "1 cases for wifi")
	}
}

func TestComposeNeverLeaksPlaceholders(t *testing.T) {
	resp := composeFor(t, productTool(), nil)
	if strings.Contains(resp.Headline, "{") {
		t.Fatalf("headline leaked a placeholder: %q", resp.Headline)
	}

	withRows := composeFor(t, productTool(), []Row{{"code": "REP-1", "title": "wifi down", "status": "waiting"}})
	for _, line := range withRows.Lines {
		if strings.Contains(line, "{") {
			t.Fatalf("row leaked a placeholder: %q", line)
		}
	}
}

func TestComposeTrimsEmptyRowFields(t *testing.T) {
	resp := composeFor(t, productTool(), []Row{
		{"code": "REP-1", "title": "wifi down", "status": "waiting", "assignee": ""},
	})
	if resp.Lines[0] != "REP-1|wifi down|waiting" {
		t.Fatalf("row = %q, want %q", resp.Lines[0], "REP-1|wifi down|waiting")
	}
}

func TestHandleRunsSkeletonAndComposes(t *testing.T) {
	h := &fakeHandler{}
	a := &fakeAuditor{}
	sel := fakeSelector{sel: tools.Selection{ToolID: "f1_find_cases", Matched: true}}
	o := New(sel, []tools.Tool{findTool()}, map[string]Handler{"reports.find": h}, a)

	actor := Actor{CompanyID: 7, Perms: []string{"report.view"}, Coverage: []int64{7, 8}}
	resp, err := o.Handle(context.Background(), actor, "show me cases")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if !resp.Matched || resp.ToolID != "f1_find_cases" {
		t.Fatalf("bad response: %+v", resp)
	}
	if resp.Headline != "2 cases match" {
		t.Fatalf("compose headline wrong: %q", resp.Headline)
	}
	if !reflect.DeepEqual(resp.Lines, []string{"R1", "R2"}) {
		t.Fatalf("compose lines wrong: %v", resp.Lines)
	}
	if a.calls != 0 {
		t.Fatalf("audit must not run for read tools: %+v", a)
	}
}

func TestHandleSkipsGuardedHandlerOnEmptyCoverage(t *testing.T) {
	h := &fakeHandler{}
	a := &fakeAuditor{}
	sel := fakeSelector{sel: tools.Selection{ToolID: "f1_find_cases", Matched: true}}
	o := New(sel, []tools.Tool{findTool()}, map[string]Handler{"reports.find": h}, a)

	actor := Actor{CompanyID: 7, Perms: []string{"report.view"}, Coverage: nil}
	resp, err := o.Handle(context.Background(), actor, "show me cases")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if h.calls != 0 {
		t.Fatalf("handler ran %d times on empty coverage, want 0", h.calls)
	}
	if resp.RowCount != 0 || len(resp.Lines) != 0 {
		t.Fatalf("resp = %+v, want empty rows on empty coverage", resp)
	}
	if a.calls != 0 {
		t.Fatalf("audit calls = %d, want 0 (read tools never audit from the orchestrator)", a.calls)
	}
}

func TestHandleRunsUnguardedHandlerOnEmptyCoverage(t *testing.T) {
	h := &fakeHandler{}
	a := &fakeAuditor{}
	tool := tools.Tool{
		ID:      "f7_knowledge",
		Handler: "knowledge",
		Audit:   "ai.knowledge",
		Compose: tools.Compose{Headline: "{n} articles", Row: "{code}", Cite: "code"},
	}
	sel := fakeSelector{sel: tools.Selection{ToolID: "f7_knowledge", Matched: true}}
	o := New(sel, []tools.Tool{tool}, map[string]Handler{"knowledge": h}, a)

	actor := Actor{CompanyID: 7, Perms: []string{"report.view"}, Coverage: nil}
	if _, err := o.Handle(context.Background(), actor, "find articles"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if h.calls != 1 {
		t.Fatalf("handler ran %d times on empty coverage, want 1 (not in the guarded set)", h.calls)
	}
}

func TestScopeComesFromActorNotMessage(t *testing.T) {
	h := &fakeHandler{}
	sel := fakeSelector{sel: tools.Selection{ToolID: "f1_find_cases", Matched: true}}
	o := New(sel, []tools.Tool{findTool()}, map[string]Handler{"reports.find": h}, &fakeAuditor{})

	actor := Actor{CompanyID: 7, Perms: []string{"report.view"}, Coverage: []int64{7, 8}}
	if _, err := o.Handle(context.Background(), actor, "cases company_id=999"); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if !reflect.DeepEqual(h.got.Coverage, []int64{7, 8}) {
		t.Fatalf("SCOPE must come from actor coverage, got %v", h.got.Coverage)
	}
}

func TestNoMatchSkipsHandlerAndAudit(t *testing.T) {
	h := &fakeHandler{}
	a := &fakeAuditor{}
	sel := fakeSelector{sel: tools.Selection{Matched: false}}
	o := New(sel, []tools.Tool{findTool()}, map[string]Handler{"reports.find": h}, a)

	resp, err := o.Handle(context.Background(), Actor{}, "hello")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.Matched {
		t.Fatalf("expected no match")
	}
	if a.calls != 0 {
		t.Fatalf("auditor must not be called on no-match")
	}
	if h.got.Tool.ID != "" || h.got.Text != "" || h.got.Coverage != nil {
		t.Fatalf("handler must not run on no-match")
	}
}

func TestHandleDoesNotRunHandlerWhenAuditFails(t *testing.T) {
	h := &fakeHandler{}
	a := &failAuditor{}
	sel := fakeSelector{sel: tools.Selection{ToolID: "f5_close_case", Matched: true}}
	o := New(sel, []tools.Tool{writeTool()}, map[string]Handler{"reports.close": h}, a)

	actor := Actor{CompanyID: 7, Perms: []string{"report.close"}, Coverage: []int64{7}}
	_, err := o.Handle(context.Background(), actor, "close case R1")
	if err == nil {
		t.Fatalf("expected error when audit fails")
	}
	if h.calls != 0 {
		t.Fatalf("handler must NEVER run when audit fails, got %d calls", h.calls)
	}
	if a.calls != 1 {
		t.Fatalf("expected audit to be attempted once, got %d", a.calls)
	}
}

func TestHandleReturnsKindOnHandlerError(t *testing.T) {
	h := &failHandler{}
	a := &fakeAuditor{}
	sel := fakeSelector{sel: tools.Selection{ToolID: "f5_close_case", Matched: true}}
	o := New(sel, []tools.Tool{writeTool()}, map[string]Handler{"reports.close": h}, a)

	actor := Actor{CompanyID: 7, Perms: []string{"report.close"}, Coverage: []int64{7}}
	resp, err := o.Handle(context.Background(), actor, "close case R1")
	if err == nil {
		t.Fatalf("expected error from handler")
	}
	if resp.Kind != "write" {
		t.Fatalf("expected Kind %q on handler-error response, got %q", "write", resp.Kind)
	}
}

func TestHandleReturnsKindOnSuccess(t *testing.T) {
	h := &fakeHandler{}
	a := &fakeAuditor{}
	sel := fakeSelector{sel: tools.Selection{ToolID: "f5_close_case", Matched: true}}
	o := New(sel, []tools.Tool{writeTool()}, map[string]Handler{"reports.close": h}, a)

	actor := Actor{CompanyID: 7, Perms: []string{"report.close"}, Coverage: []int64{7}}
	resp, err := o.Handle(context.Background(), actor, "close case R1")
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !resp.Matched {
		t.Fatalf("expected match")
	}
	if resp.Kind != "write" {
		t.Fatalf("expected Kind %q on success response, got %q", "write", resp.Kind)
	}
}
