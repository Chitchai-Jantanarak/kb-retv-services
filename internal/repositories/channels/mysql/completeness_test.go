package mysql

import (
	"context"
	"database/sql"
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
		!strings.Contains(q.execSQL, "intake_fields") ||
		!strings.Contains(q.execSQL, "intake_score") ||
		!strings.Contains(q.execSQL, "intake_reasons") {
		t.Fatalf("SQL = %s", q.execSQL)
	}
	if len(q.execArgs) != 9 {
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
	if q.execArgs[8] != int64(42) {
		t.Fatalf("conversation arg = %v", q.execArgs[8])
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

func TestWriteCompletenessPersistsClassificationAndCatalogRelated(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)

	related := false
	err := repo.WriteCompleteness(context.Background(), 42, intake.Result{
		Status:         intake.StatusReady,
		Classification: intake.ClassificationNewIssue,
		CatalogRelated: &related,
	})
	if err != nil {
		t.Fatalf("WriteCompleteness() error = %v", err)
	}
	if !strings.Contains(q.execSQL, "intake_classification") || !strings.Contains(q.execSQL, "intake_catalog_related") {
		t.Fatalf("SQL must set intake_classification and intake_catalog_related, got: %s", q.execSQL)
	}
	classification, ok := q.execArgs[5].(sql.NullString)
	if !ok || !classification.Valid || classification.String != intake.ClassificationNewIssue {
		t.Fatalf("classification arg = %v", q.execArgs[5])
	}
	catalogRelated, ok := q.execArgs[6].(sql.NullBool)
	if !ok || !catalogRelated.Valid || catalogRelated.Bool != false {
		t.Fatalf("catalog_related arg = %v", q.execArgs[6])
	}
}

func TestWriteCompletenessWritesNullForUnsetClassificationAndCatalogRelated(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)

	err := repo.WriteCompleteness(context.Background(), 42, intake.Result{Status: intake.StatusReady})
	if err != nil {
		t.Fatalf("WriteCompleteness() error = %v", err)
	}
	classification, ok := q.execArgs[5].(sql.NullString)
	if !ok || classification.Valid {
		t.Fatalf("classification arg = %v, want SQL NULL for empty classification", q.execArgs[5])
	}
	catalogRelated, ok := q.execArgs[6].(sql.NullBool)
	if !ok || catalogRelated.Valid {
		t.Fatalf("catalog_related arg = %v, want SQL NULL for nil CatalogRelated", q.execArgs[6])
	}
}

func TestWriteCompletenessPersistsReferencedCase(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)

	err := repo.WriteCompleteness(context.Background(), 42, intake.Result{
		Status:         intake.StatusReady,
		ReferencedCase: "REP-4106",
	})
	if err != nil {
		t.Fatalf("WriteCompleteness() error = %v", err)
	}
	if !strings.Contains(q.execSQL, "intake_referenced_case") {
		t.Fatalf("SQL must set intake_referenced_case, got: %s", q.execSQL)
	}
	referencedCase, ok := q.execArgs[7].(sql.NullString)
	if !ok || !referencedCase.Valid || referencedCase.String != "REP-4106" {
		t.Fatalf("referenced_case arg = %v", q.execArgs[7])
	}
}

func TestWriteCompletenessWritesNullForEmptyReferencedCase(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)

	err := repo.WriteCompleteness(context.Background(), 42, intake.Result{Status: intake.StatusReady})
	if err != nil {
		t.Fatalf("WriteCompleteness() error = %v", err)
	}
	referencedCase, ok := q.execArgs[7].(sql.NullString)
	if !ok || referencedCase.Valid {
		t.Fatalf("referenced_case arg = %v, want SQL NULL for empty ReferencedCase", q.execArgs[7])
	}
}
