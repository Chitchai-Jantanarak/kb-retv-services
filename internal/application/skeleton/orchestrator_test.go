package skeleton

import (
	"context"
	"reflect"
	"testing"

	"github.com/my/app/internal/application/tools"
)

type fakeSelector struct{ sel tools.Selection }

func (f fakeSelector) Select(_ context.Context, _ string, _ []string) (tools.Selection, error) {
	return f.sel, nil
}

type fakeHandler struct{ got Query }

func (f *fakeHandler) Run(_ context.Context, q Query) ([]Row, error) {
	f.got = q
	return []Row{{"code": "R1"}, {"code": "R2"}}, nil
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
	if a.calls != 1 || a.tag != "ai.find_cases" {
		t.Fatalf("audit not logged correctly: %+v", a)
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
