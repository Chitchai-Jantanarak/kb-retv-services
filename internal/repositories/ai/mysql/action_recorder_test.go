package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/my/app/internal/domain/ports"
)

func TestActionRecorderValidatesInput(t *testing.T) {
	rec := NewActionRecorder(nil)
	cases := []struct {
		name    string
		action  ports.AIAction
		wantSub string
	}{
		{name: "zero_company", action: ports.AIAction{AgentID: 1, ActionType: "x"}, wantSub: "company_id"},
		{name: "zero_agent", action: ports.AIAction{CompanyID: 1, ActionType: "x"}, wantSub: "ai_agent_id"},
		{name: "missing_type", action: ports.AIAction{CompanyID: 1, AgentID: 1, ActionType: "   "}, wantSub: "action_type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := rec.Record(context.Background(), tc.action)
			if err == nil {
				t.Fatalf("err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want sub %q", err.Error(), tc.wantSub)
			}
		})
	}
}
