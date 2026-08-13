package mysql

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

type capturingQuerier struct {
	lastQuery string
	lastArgs  []any
}

func (c *capturingQuerier) QueryContext(_ context.Context, q string, args ...any) (*sql.Rows, error) {
	c.lastQuery, c.lastArgs = q, args
	return nil, sql.ErrNoRows
}

func (c *capturingQuerier) QueryRowContext(_ context.Context, q string, args ...any) *sql.Row {
	c.lastQuery, c.lastArgs = q, args
	return nil
}

func (c *capturingQuerier) ExecContext(_ context.Context, q string, args ...any) (sql.Result, error) {
	c.lastQuery, c.lastArgs = q, args
	return nil, nil
}

func (c *capturingQuerier) BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error) {
	return nil, nil
}

func TestLastCitesQueryScopesToCompanyAndUser(t *testing.T) {
	if !strings.Contains(lastCitesQuery, "s.company_id = ?") {
		t.Fatalf("query = %q, want a company_id predicate", lastCitesQuery)
	}
	if !strings.Contains(lastCitesQuery, "s.user_id = ?") {
		t.Fatalf("query = %q, want a user_id predicate; a session belongs to whoever opened it", lastCitesQuery)
	}
	if !strings.Contains(lastCitesQuery, "t.role = 'assistant'") {
		t.Fatalf("query = %q, want only assistant turns to supply cite candidates", lastCitesQuery)
	}
	if !strings.Contains(lastCitesQuery, "ORDER BY t.id DESC") || !strings.Contains(lastCitesQuery, "LIMIT 1") {
		t.Fatalf("query = %q, want the most recent citing turn only", lastCitesQuery)
	}
}

func TestSessionLookupQueryScopesToCompanyAndUser(t *testing.T) {
	for _, want := range []string{"id = ?", "company_id = ?", "user_id = ?"} {
		if !strings.Contains(sessionLookupQuery, want) {
			t.Fatalf("query = %q, missing %q", sessionLookupQuery, want)
		}
	}
}

func TestLastCitesReturnsNothingWithoutScope(t *testing.T) {
	db := &capturingQuerier{}
	repo := New(db)

	key, values, err := repo.LastCites(context.Background(), 0, 77, 9001)
	if err != nil || key != "" || values != nil {
		t.Fatalf("got (%q, %v, %v), want empty with no error when company scope is absent", key, values, err)
	}
	if db.lastQuery != "" {
		t.Fatalf("query = %q, want no statement issued without scope", db.lastQuery)
	}
}

func TestAppendTurnRejectsUnknownRole(t *testing.T) {
	db := &capturingQuerier{}
	repo := New(db)

	if err := repo.AppendTurn(context.Background(), Turn{SessionID: 1, Role: "system"}); err == nil {
		t.Fatal("want an error: role is an enum of user|assistant in the schema")
	}
	if db.lastQuery != "" {
		t.Fatalf("query = %q, want no statement issued for an invalid role", db.lastQuery)
	}
}

func TestAppendTurnSerialisesCiteValues(t *testing.T) {
	db := &capturingQuerier{}
	repo := New(db)

	if err := repo.AppendTurn(context.Background(), Turn{
		SessionID: 9001,
		Role:      "assistant",

		CiteKey:    "code",
		CiteValues: []string{"REP-4106", "REP-4105"},
	}); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}
	if !strings.Contains(db.lastQuery, "chat_sessions SET last_turn_at") {
		t.Fatalf("last query = %q, want the session touched after the insert", db.lastQuery)
	}
}

func TestEnsureSessionRequiresScope(t *testing.T) {
	db := &capturingQuerier{}
	repo := New(db)

	if _, err := repo.EnsureSession(context.Background(), 0, 77, 0); err == nil {
		t.Fatal("want an error when company scope is missing")
	}
	if _, err := repo.EnsureSession(context.Background(), 4, 0, 0); err == nil {
		t.Fatal("want an error when user scope is missing")
	}
	if db.lastQuery != "" {
		t.Fatalf("query = %q, want no statement issued without scope", db.lastQuery)
	}
}
