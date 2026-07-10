package toolaudit

import (
	"context"
	"testing"

	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/domain/ports"
)

type recRec struct{ got ports.AIAction }

func (r *recRec) Record(_ context.Context, a ports.AIAction) (int64, error) { r.got = a; return 1, nil }

func TestAuditorLogRecordsAction(t *testing.T) {
	t.Parallel()
	rec := &recRec{}
	a := New(rec, func(context.Context, int64) (int64, bool) { return 7, true })
	if err := a.Log(context.Background(), "ai.update_case", skeleton.Actor{CompanyID: 3}, "f8_update_case"); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if rec.got.CompanyID != 3 || rec.got.AgentID != 7 || rec.got.ActionType != "ai.update_case" || rec.got.Input != "f8_update_case" {
		t.Fatalf("recorded = %+v", rec.got)
	}
}

func TestAuditorLogFailsWithoutAgent(t *testing.T) {
	t.Parallel()
	rec := &recRec{}
	a := New(rec, func(context.Context, int64) (int64, bool) { return 0, false })
	if err := a.Log(context.Background(), "ai.update_case", skeleton.Actor{CompanyID: 3}, "f8_update_case"); err == nil {
		t.Fatal("expected error when no agent resolves")
	}
}
