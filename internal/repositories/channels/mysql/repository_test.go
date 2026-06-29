package mysql

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/my/app/internal/application/workflows/omnichannel"
)

type fakeResult struct{ id int64 }

func (f fakeResult) LastInsertId() (int64, error) { return f.id, nil }
func (f fakeResult) RowsAffected() (int64, error) { return 1, nil }

type fakeQuerier struct {
	execSQL  string
	execArgs []any
}

func (f *fakeQuerier) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, nil
}
func (f *fakeQuerier) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }
func (f *fakeQuerier) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	f.execSQL = query
	f.execArgs = args
	return fakeResult{id: 42}, nil
}
func (f *fakeQuerier) BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error) { return nil, nil }

func TestUpsertConversationStoresSubject(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)
	if _, _, err := repo.UpsertConversation(context.Background(), omnichannel.Conversation{
		CompanyID:        7,
		ChannelAccountID: 11,
		ExternalCustomer: "a@x.com",
		Subject:          "printer broken",
		ForceNew:         true,
	}); err != nil {
		t.Fatalf("UpsertConversation: %v", err)
	}
	if !strings.Contains(q.execSQL, "subject") {
		t.Fatalf("INSERT must include subject column, got: %s", q.execSQL)
	}
	found := false
	for _, a := range q.execArgs {
		if a == "printer broken" {
			found = true
		}
	}
	if !found {
		t.Fatalf("subject value not passed as arg: %v", q.execArgs)
	}
}

func TestUpsertConversationInsertsPendingDraft(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)

	id, created, err := repo.UpsertConversation(context.Background(), omnichannel.Conversation{
		CompanyID:        7,
		ChannelAccountID: 11,
		ExternalCustomer: "acme@example.com",
		ForceNew:         true,
	})
	if err != nil {
		t.Fatalf("UpsertConversation: %v", err)
	}
	if id != 42 || !created {
		t.Fatalf("id=%d created=%v, want 42/true", id, created)
	}
	if !strings.Contains(q.execSQL, "'pending'") {
		t.Fatalf("draft insert must set status 'pending', got SQL: %s", q.execSQL)
	}
	if strings.Contains(q.execSQL, "'open'") {
		t.Fatalf("draft insert must not use legacy status 'open': %s", q.execSQL)
	}
}
