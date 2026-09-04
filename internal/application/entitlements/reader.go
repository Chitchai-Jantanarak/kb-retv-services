package entitlements

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"time"
)

type RowScanner interface {
	Scan(dest ...any) error
}

type RowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
}

type SQLDB struct{ DB *sql.DB }

func (s SQLDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	return s.DB.QueryRowContext(ctx, query, args...)
}

type Reader struct {
	db    RowQuerier
	ttl   time.Duration
	mu    sync.Mutex
	cache map[int64]entry
}

type entry struct {
	flags   map[string]json.RawMessage
	expires time.Time
}

func New(db RowQuerier, ttl time.Duration) *Reader {
	return &Reader{db: db, ttl: ttl, cache: map[int64]entry{}}
}

func (r *Reader) Enabled(ctx context.Context, companyID int64, key string) bool {
	flags := r.flags(ctx, companyID)
	raw, ok := flags[key]
	if !ok {
		return true
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return true
	}
	return b
}

func (r *Reader) flags(ctx context.Context, companyID int64) map[string]json.RawMessage {
	if r.ttl > 0 {
		r.mu.Lock()
		e, ok := r.cache[companyID]
		r.mu.Unlock()
		if ok && time.Now().Before(e.expires) {
			return e.flags
		}
	}
	var col sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT entitlements FROM companies WHERE id = ? LIMIT 1", companyID).Scan(&col)
	if err != nil {
		return nil
	}
	flags := map[string]json.RawMessage{}
	if col.Valid && col.String != "" {
		_ = json.Unmarshal([]byte(col.String), &flags)
	}
	if r.ttl > 0 {
		r.mu.Lock()
		r.cache[companyID] = entry{flags: flags, expires: time.Now().Add(r.ttl)}
		r.mu.Unlock()
	}
	return flags
}
