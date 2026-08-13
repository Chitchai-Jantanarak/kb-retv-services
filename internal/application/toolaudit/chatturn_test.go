package toolaudit

import (
	"context"
	"testing"

	chatwf "github.com/my/app/internal/application/workflows/chat"
)

func TestChatTurnAuditorWritesDecisionCounters(t *testing.T) {
	t.Parallel()
	rec := &recRec{}
	a := NewChatTurn(rec, func(context.Context, int64) (int64, bool) { return 7, true })

	err := a.RecordChatTurn(context.Background(), 3, chatwf.TurnAudit{
		Outcome:       "answered",
		ToolID:        "f1_find_cases",
		DecisionKind:  "tool",
		Score:         0.82,
		RunnerUpScore: 0.61,
		Margin:        0.21,
		RowsReturned:  2,
		Lang:          "th",
		Audience:      "employee",
	})
	if err != nil {
		t.Fatalf("RecordChatTurn: %v", err)
	}

	input, ok := rec.got.Input.(map[string]any)
	if !ok {
		t.Fatalf("Input = %+v, want map[string]any", rec.got.Input)
	}
	if input["decision_kind"] != "tool" || input["lang"] != "th" || input["audience"] != "employee" {
		t.Fatalf("Input = %+v, missing decision counters", input)
	}

	output, ok := rec.got.Output.(map[string]any)
	if !ok {
		t.Fatalf("Output = %+v, want map[string]any", rec.got.Output)
	}
	if output["score"] != 0.82 || output["runner_up_score"] != 0.61 || output["margin"] != 0.21 || output["rows_returned"] != 2 {
		t.Fatalf("Output = %+v, missing decision counters", output)
	}
}
