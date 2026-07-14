package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/my/app/internal/application/workflows/intake"
)

func TestWriteCompletenessPersistsStatusMissingAndFields(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)

	err := repo.WriteCompleteness(context.Background(), 42, intake.Result{
		Fields:  map[string]string{"problem_detail": "stuck at dock"},
		Missing: []string{"product"},
		Status:  intake.StatusIncomplete,
	})
	if err != nil {
		t.Fatalf("WriteCompleteness() error = %v", err)
	}
	if !strings.Contains(q.execSQL, "UPDATE conversations") ||
		!strings.Contains(q.execSQL, "intake_status") ||
		!strings.Contains(q.execSQL, "intake_missing") ||
		!strings.Contains(q.execSQL, "intake_fields") {
		t.Fatalf("SQL = %s", q.execSQL)
	}
	if len(q.execArgs) != 4 {
		t.Fatalf("args = %v", q.execArgs)
	}
	if q.execArgs[0] != intake.StatusIncomplete {
		t.Fatalf("status arg = %v", q.execArgs[0])
	}
	if q.execArgs[1] != `["product"]` {
		t.Fatalf("missing arg = %v", q.execArgs[1])
	}
	if q.execArgs[2] != `{"problem_detail":"stuck at dock"}` {
		t.Fatalf("fields arg = %v", q.execArgs[2])
	}
	if q.execArgs[3] != int64(42) {
		t.Fatalf("conversation arg = %v", q.execArgs[3])
	}
}

func TestWriteCompletenessEncodesEmptyCollectionsAsJSON(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)

	if err := repo.WriteCompleteness(context.Background(), 9, intake.Result{Status: intake.StatusReady}); err != nil {
		t.Fatalf("WriteCompleteness() error = %v", err)
	}
	if q.execArgs[1] != `[]` || q.execArgs[2] != `{}` {
		t.Fatalf("empty collections must encode as [] and {}, got %v / %v", q.execArgs[1], q.execArgs[2])
	}
}

func TestWriteCompletenessRejectsZeroConversation(t *testing.T) {
	repo := New(&fakeQuerier{})
	if err := repo.WriteCompleteness(context.Background(), 0, intake.Result{Status: intake.StatusReady}); err == nil {
		t.Fatal("WriteCompleteness() error = nil, want conversation_id error")
	}
}
