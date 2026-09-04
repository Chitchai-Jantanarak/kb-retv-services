package entitlements

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

type fakeRow struct {
	val sql.NullString
	err error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*sql.NullString)) = r.val
	return nil
}

type fakeDB struct {
	calls int
	row   fakeRow
}

func (f *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	f.calls++
	return f.row
}

func TestMissingKeyAndNullColumnAreAllowed(t *testing.T) {
	db := &fakeDB{row: fakeRow{val: sql.NullString{Valid: false}}}
	r := New(db, time.Minute)
	if !r.Enabled(context.Background(), 7, "feature.ai.enabled") {
		t.Fatal("null column must allow")
	}
	db.row = fakeRow{val: sql.NullString{String: `{"feature.knowledge.semantic_search":false}`, Valid: true}}
	r2 := New(db, 0)
	if !r2.Enabled(context.Background(), 7, "feature.ai.enabled") {
		t.Fatal("absent key must allow")
	}
	if r2.Enabled(context.Background(), 7, "feature.knowledge.semantic_search") {
		t.Fatal("explicit false must gate")
	}
}

func TestQueryErrorAllowsAndCacheAvoidsSecondQuery(t *testing.T) {
	db := &fakeDB{row: fakeRow{err: errors.New("boom")}}
	r := New(db, time.Minute)
	if !r.Enabled(context.Background(), 1, "feature.ai.enabled") {
		t.Fatal("db error must fail open")
	}
	db.row = fakeRow{val: sql.NullString{String: `{"feature.ai.enabled":false}`, Valid: true}}
	r = New(db, time.Minute)
	r.Enabled(context.Background(), 1, "feature.ai.enabled")
	r.Enabled(context.Background(), 1, "feature.channels.line")
	if db.calls != 2 {
		t.Fatalf("expected 2 total queries (1 error + 1 cached), got %d", db.calls)
	}
}
