package reply

import (
	"context"
	"sync"
	"testing"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/domain/kb"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/repositories/kb/memory"
	"github.com/my/app/internal/shared/ctxkey"
)

type captureRecorder struct {
	mu      sync.Mutex
	actions []ports.AIAction
	err     error
}

func (r *captureRecorder) Record(_ context.Context, a ports.AIAction) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions = append(r.actions, a)
	return int64(len(r.actions)), r.err
}

func newKnowledge() ports.KnowledgeRepository {
	return memory.NewKnowledgeRepository([]kb.Article{
		{ID: "kb-1", CompanyID: 7, Title: "Printer offline", Summary: "Restart the printer."},
	})
}

func TestRecordActionSkippedWhenNoAgent(t *testing.T) {
	rec := &captureRecorder{}
	wf := NewWorkflow(
		newKnowledge(),
		WithActionRecorder(rec, func(_ context.Context, _ int64) (int64, bool) { return 0, false }),
	)
	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	_, err := wf.Run(ctx, dto.ReplyRequest{CustomerID: "cust-1", Message: "printer offline", Channel: "web"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.actions) != 0 {
		t.Fatalf("actions = %d, want 0 (agent_id missing)", len(rec.actions))
	}
}

func TestRecordActionEmitsRow(t *testing.T) {
	rec := &captureRecorder{}
	wf := NewWorkflow(
		newKnowledge(),
		WithActionRecorder(rec, func(_ context.Context, _ int64) (int64, bool) { return 42, true }),
	)
	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	_, err := wf.Run(ctx, dto.ReplyRequest{CustomerID: "cust-1", Message: "printer offline", Channel: "web"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.actions) != 1 {
		t.Fatalf("actions = %d, want 1", len(rec.actions))
	}
	got := rec.actions[0]
	if got.CompanyID != 7 || got.AgentID != 42 {
		t.Fatalf("identity wrong: %+v", got)
	}
	if got.ActionType != "smart_reply" {
		t.Fatalf("action_type = %q, want smart_reply", got.ActionType)
	}
}
