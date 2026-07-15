package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/intent"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
)

func TestAuthoritativeIntentClassification(t *testing.T) {
	for _, i := range []intent.Intent{intent.CaseStatus, intent.OpenCase} {
		if !authoritativeIntent(i) {
			t.Fatalf("%q must be authoritative", i)
		}
	}
	for _, i := range []intent.Intent{intent.KBSearch, intent.GeneralSupport, intent.Handoff, intent.OffDomain} {
		if authoritativeIntent(i) {
			t.Fatalf("%q must not be authoritative", i)
		}
	}
}

func TestRunCaseStatusMatchedReturnsToolRows(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"should not run","case":null}`}
	fts := &stubFTS{}
	orch := stubOrch{resp: skeleton.Response{Matched: true, ToolID: "f2_track_case", Headline: "1 case", Lines: []string{"CASE-1|open"}}}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)), WithOrchestrator(orch))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "ตรวจสอบสถานะเคส"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0 for case_status match (never reaches LLM)", provider.calls)
	}
	if resp.Status != dto.ChatStatusAnswered {
		t.Fatalf("status = %q, want answered", resp.Status)
	}
	if !strings.Contains(resp.Reply, "1 case") {
		t.Fatalf("reply = %q, want the tool headline", resp.Reply)
	}
}

func TestRunCaseStatusToolErrorHandsOff(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"should not run","case":null}`}
	fts := &stubFTS{}
	orch := stubOrch{err: apperr.New(apperr.CodeInternal, "handler missing")}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)), WithOrchestrator(orch))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "ตรวจสอบสถานะเคส"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0 (authoritative intent must not reach LLM)", provider.calls)
	}
	if resp.Status != dto.ChatStatusHandoff {
		t.Fatalf("status = %q, want handoff on authoritative tool error", resp.Status)
	}
}
