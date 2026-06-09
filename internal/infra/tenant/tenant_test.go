package tenant

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"

	_ "github.com/go-sql-driver/mysql"
)

func lazyDB(t *testing.T, name string) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", "u:p@tcp(127.0.0.1:3306)/"+name)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}

func TestNewPoolValidation(t *testing.T) {
	if _, err := NewPool(nil, func(string) (*sql.DB, error) { return nil, nil }); err == nil {
		t.Fatal("expected error for nil manager")
	}
	mgr := lazyDB(t, "mgr")
	defer mgr.Close()
	if _, err := NewPool(mgr, nil); err == nil {
		t.Fatal("expected error for nil open func")
	}
}

func TestPoolGetCachesPerTenant(t *testing.T) {
	mgr := lazyDB(t, "mgr")
	defer mgr.Close()

	var mu sync.Mutex
	opened := map[string]int{}
	open := func(name string) (*sql.DB, error) {
		mu.Lock()
		opened[name]++
		mu.Unlock()
		return lazyDB(t, name), nil
	}
	pool, err := NewPool(mgr, open)
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}
	defer pool.Close()

	a1, err := pool.get("tenant_a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	a2, _ := pool.get("tenant_a")
	b1, _ := pool.get("tenant_b")

	if a1 != a2 {
		t.Fatal("expected cached handle reuse for tenant_a")
	}
	if a1 == b1 {
		t.Fatal("expected distinct handle for tenant_b")
	}
	if opened["tenant_a"] != 1 {
		t.Fatalf("tenant_a opened %d times, want 1", opened["tenant_a"])
	}
	if opened["tenant_b"] != 1 {
		t.Fatalf("tenant_b opened %d times, want 1", opened["tenant_b"])
	}
}

func TestPoolGetPropagatesOpenError(t *testing.T) {
	mgr := lazyDB(t, "mgr")
	defer mgr.Close()
	pool, _ := NewPool(mgr, func(string) (*sql.DB, error) {
		return nil, errFakeOpen
	})
	_, err := pool.get("tenant_x")
	if err == nil || !strings.Contains(err.Error(), "open") {
		t.Fatalf("err = %v, want open failure", err)
	}
}

func TestPoolConnForRejectsMissingCompanyID(t *testing.T) {
	mgr := lazyDB(t, "mgr")
	defer mgr.Close()
	pool, err := NewPool(mgr, func(name string) (*sql.DB, error) {
		return lazyDB(t, name), nil
	})
	if err != nil {
		t.Fatalf("NewPool: %v", err)
	}

	if _, err := pool.connFor(context.Background()); err == nil || !strings.Contains(err.Error(), "company_id") {
		t.Fatalf("connFor err = %v, want missing company_id error", err)
	}
}

var errFakeOpen = errOpen("boom")

type errOpen string

func (e errOpen) Error() string { return string(e) }
