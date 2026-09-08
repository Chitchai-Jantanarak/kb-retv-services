package mysql

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/my/app/internal/gmailconn"
)

func TestSaveHistoryIDRequiresPositiveID(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)
	if err := repo.SaveHistoryID(context.Background(), 0, "123"); err == nil {
		t.Fatal("expected error for id <= 0")
	}
	if q.execSQL != "" {
		t.Fatal("must not exec before validating id")
	}
}

func TestSaveHistoryIDRequiresNonEmptyValue(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)
	if err := repo.SaveHistoryID(context.Background(), 1, "   "); err == nil {
		t.Fatal("expected error for a blank history id")
	}
	if q.execSQL != "" {
		t.Fatal("must not exec before validating history id")
	}
}

func TestSaveHistoryIDIssuesJSONSetUpdate(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)
	if err := repo.SaveHistoryID(context.Background(), 1, "999"); err != nil {
		t.Fatalf("SaveHistoryID: %v", err)
	}
	if !strings.Contains(q.execSQL, "JSON_SET") || !strings.Contains(q.execSQL, "oauth_history_id") {
		t.Fatalf("expected a JSON_SET update on oauth_history_id, got: %s", q.execSQL)
	}
}

func TestMarkOAuthStateRequiresPositiveID(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)
	if err := repo.MarkOAuthState(context.Background(), 0, gmailconn.OAuthStateOK); err == nil {
		t.Fatal("expected error for id <= 0")
	}
	if q.execSQL != "" {
		t.Fatal("must not exec before validating id")
	}
}

func TestMarkOAuthStateRejectsUnknownValue(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)
	if err := repo.MarkOAuthState(context.Background(), 1, "bogus"); err == nil {
		t.Fatal("expected error for an unrecognized oauth state")
	}
	if q.execSQL != "" {
		t.Fatal("must not exec an unrecognized state")
	}
}

func TestMarkOAuthStateAcceptsKnownValues(t *testing.T) {
	for _, state := range []string{gmailconn.OAuthStateOK, gmailconn.OAuthStateNeedsReconnect} {
		q := &fakeQuerier{}
		repo := New(q)
		if err := repo.MarkOAuthState(context.Background(), 1, state); err != nil {
			t.Fatalf("MarkOAuthState(%q): %v", state, err)
		}
		if !strings.Contains(q.execSQL, "oauth_state") {
			t.Fatalf("expected update on oauth_state, got: %s", q.execSQL)
		}
	}
}

// queryErrQuerier is a minimal tenant.Querier that always fails QueryContext,
// used to check ListOAuthGmail's error path without a real *sql.Rows (the
// shared fakeQuerier in repository_test.go returns (nil, nil), which would
// panic on the rows.Close()/rows.Next() calls ListOAuthGmail makes on a real
// result set).
type queryErrQuerier struct{ err error }

func (q queryErrQuerier) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, q.err
}
func (q queryErrQuerier) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }
func (q queryErrQuerier) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, nil
}
func (q queryErrQuerier) BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error) { return nil, nil }

func TestListOAuthGmailPropagatesQueryError(t *testing.T) {
	wantErr := errors.New("boom")
	repo := New(queryErrQuerier{err: wantErr})
	if _, err := repo.ListOAuthGmail(context.Background()); err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want wrapping %v", err, wantErr)
	}
}
