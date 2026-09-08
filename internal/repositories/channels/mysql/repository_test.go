package mysql

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/workflows/omnichannel"
)

type fakeResult struct{ id int64 }

func (f fakeResult) LastInsertId() (int64, error) { return f.id, nil }
func (f fakeResult) RowsAffected() (int64, error) { return 1, nil }

type fakeQuerier struct {
	execSQL     string
	execArgs    []any
	queryCalled bool
}

func (f *fakeQuerier) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	f.queryCalled = true
	return nil, nil
}
func (f *fakeQuerier) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }
func (f *fakeQuerier) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	f.execSQL = query
	f.execArgs = args
	return fakeResult{id: 42}, nil
}
func (f *fakeQuerier) BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error) { return nil, nil }

func TestFindByExternalIDRequiresCompany(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)
	if _, _, _, err := repo.FindByExternalID(context.Background(), "ext-1"); err == nil {
		t.Fatal("FindByExternalID must reject a context with no company_id")
	}
	if q.queryCalled {
		t.Fatal("FindByExternalID must not query before the company scope is known")
	}
}

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

func TestInsertMessageDoesNotWriteAttachmentsColumn(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)

	if _, err := repo.InsertMessage(context.Background(), omnichannel.StoredMessage{
		ConversationID: 5,
		Body:           "hello",
		Attachments:    []dto.AttachmentRef{{StorageKey: "s3://bucket/key.png"}},
	}); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}
	if strings.Contains(q.execSQL, "attachments") {
		t.Fatalf("INSERT must not reference attachments column, got: %s", q.execSQL)
	}
	for _, a := range q.execArgs {
		if s, ok := a.(string); ok && strings.Contains(s, "s3://bucket/key.png") {
			t.Fatalf("attachment payload must not be passed as insert arg: %v", q.execArgs)
		}
	}
}

func TestInsertMessageWritesNullAttachmentsWhenEmpty(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)

	if _, err := repo.InsertMessage(context.Background(), omnichannel.StoredMessage{
		ConversationID: 5,
		SenderExternal: "cust-1",
		Body:           "hello",
	}); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}
	if len(q.execArgs) != 5 {
		t.Fatalf("expected 5 insert args, got %d in %v", len(q.execArgs), q.execArgs)
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

func TestMarkVerifiedRequiresChannelAndCode(t *testing.T) {
	cases := []struct {
		name    string
		channel string
		code    string
	}{
		{name: "empty_channel", channel: "", code: "AB23CD45"},
		{name: "empty_code", channel: "email", code: ""},
		{name: "both_empty", channel: "", code: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeQuerier{}
			repo := New(q)
			if _, err := repo.MarkVerified(context.Background(), tc.channel, tc.code); err == nil {
				t.Fatal("MarkVerified must reject a missing channel or verification code")
			}
			if q.execSQL != "" {
				t.Fatalf("MarkVerified must not touch the db before validation, got exec: %s", q.execSQL)
			}
		})
	}
}

func TestTouchInboundRequiresAccountID(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)
	if err := repo.TouchInbound(context.Background(), 0); err == nil {
		t.Fatal("TouchInbound must reject a zero account_id")
	}
	if q.execSQL != "" {
		t.Fatalf("TouchInbound must not touch the db before validation, got exec: %s", q.execSQL)
	}
	if err := repo.TouchInbound(context.Background(), -1); err == nil {
		t.Fatal("TouchInbound must reject a negative account_id")
	}
}

// ByRoutingKey/ByAlias must reject an empty key/address before reaching
// QueryRowContext: fakeQuerier.QueryRowContext returns a nil *sql.Row, so a
// validation gap here would panic on Scan instead of returning a clean error.

func TestByRoutingKeyRequiresKey(t *testing.T) {
	repo := New(&fakeQuerier{})
	if _, err := repo.ByRoutingKey(context.Background(), ""); err == nil {
		t.Fatal("ByRoutingKey must reject an empty routing_key")
	}
	if _, err := repo.ByRoutingKey(context.Background(), "   "); err == nil {
		t.Fatal("ByRoutingKey must reject a blank routing_key")
	}
}

func TestByAliasRequiresAddress(t *testing.T) {
	repo := New(&fakeQuerier{})
	if _, err := repo.ByAlias(context.Background(), ""); err == nil {
		t.Fatal("ByAlias must reject an empty address")
	}
	if _, err := repo.ByAlias(context.Background(), "   "); err == nil {
		t.Fatal("ByAlias must reject a blank address")
	}
}

func TestDeleteConversationIfEmptyIsTenantScopedAndConservative(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)

	if err := repo.DeleteConversationIfEmpty(context.Background(), 7, 42); err != nil {
		t.Fatalf("DeleteConversationIfEmpty: %v", err)
	}
	for _, fragment := range []string{"c.company_id = ?", "c.status = 'pending'", "c.report_id IS NULL", "m.id IS NULL"} {
		if !strings.Contains(q.execSQL, fragment) {
			t.Fatalf("delete must contain %q, got: %s", fragment, q.execSQL)
		}
	}
	if len(q.execArgs) != 2 || q.execArgs[0] != int64(42) || q.execArgs[1] != int64(7) {
		t.Fatalf("delete args = %v, want [42 7]", q.execArgs)
	}
}
