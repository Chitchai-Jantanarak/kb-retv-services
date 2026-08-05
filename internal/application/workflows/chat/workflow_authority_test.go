package chat

import (
	"context"
	"strings"
	"testing"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/ai/rag"
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

func TestActivityReflectsExecutedStages(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"unused","case":null}`}
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "KB", Content: "x", Relevance: 7.5}}}
	orch := stubOrch{resp: skeleton.Response{Matched: true, ToolID: "f1_find_cases", ComposeMode: "template", Headline: "1 cases match", Lines: []string{"REP-1"}}}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)), WithOrchestrator(orch))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "robot broken"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	codes := map[string]bool{}
	for _, a := range resp.Activity {
		codes[a.Code] = true
	}
	if !codes["used_tool"] {
		t.Fatalf("tool turn missing used_tool: %v", resp.Activity)
	}
	if codes["searched_knowledge"] {
		t.Fatalf("tool turn falsely claims searched_knowledge: %v", resp.Activity)
	}
}

type countingFTS struct {
	calls int
}

func (c *countingFTS) SearchChunks(context.Context, []int64, string, int) ([]rag.FTSChunk, error) {
	c.calls++
	return nil, nil
}

func TestRunSkipsKnowledgeOnAuthoritativeIntent(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"should not run","case":null}`}
	fts := &countingFTS{}
	orch := stubOrch{resp: skeleton.Response{Matched: false}}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)), WithOrchestrator(orch))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	before := fts.calls
	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "ตรวจสอบสถานะเคส"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Status != dto.ChatStatusNeedsClarification {
		t.Fatalf("status = %q, want needs_clarification", resp.Status)
	}
	if len(resp.Sources) != 0 {
		t.Fatalf("authoritative turn attached sources: %+v", resp.Sources)
	}
	if fts.calls != before {
		t.Fatalf("knowledge FTS ran %d times on authoritative turn", fts.calls-before)
	}
	if provider.calls != 0 {
		t.Fatalf("LLM ran on authoritative turn")
	}
}

func TestRunPermissionDeniedWhenAllToolsFiltered(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"should not run","case":null}`}
	fts := &stubFTS{}
	orch := stubOrch{resp: skeleton.Response{Matched: false, PermFiltered: true}}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)), WithOrchestrator(orch))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "ตรวจสอบสถานะเคส"}},
		Locale:   "en",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Status != dto.ChatStatusPermissionDenied {
		t.Fatalf("status = %q, want permission_denied", resp.Status)
	}
	if strings.Contains(resp.Reply, "staff member") || strings.Contains(resp.Reply, "which case") {
		t.Fatalf("reply = %q, want a permission message", resp.Reply)
	}
}

func TestRunCaseStatusToolErrorReportsToolFailed(t *testing.T) {
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
	if resp.Status != dto.ChatStatusToolFailed {
		t.Fatalf("status = %q, want tool_failed on authoritative tool error", resp.Status)
	}
}
