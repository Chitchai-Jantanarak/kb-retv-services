package budget

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeCache struct {
	mu    sync.Mutex
	items map[string][]byte
}

func newFakeCache() *fakeCache {
	return &fakeCache{items: make(map[string][]byte)}
}

func (c *fakeCache) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.items[key], nil
}

func (c *fakeCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = value
	return nil
}

func (c *fakeCache) Delete(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.items, k)
	}
	return nil
}

func (c *fakeCache) SetNX(_ context.Context, key string, value []byte, _ time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; ok {
		return false, nil
	}
	c.items[key] = value
	return true, nil
}

func TestLimiterPerRequestCeiling(t *testing.T) {
	l := New(Config{MaxCallsPerRequest: 2})
	ctx := l.Begin(context.Background())
	for i, want := range []bool{true, true, false, false} {
		got, err := l.Acquire(ctx, 1, "stage")
		if err != nil {
			t.Fatalf("Acquire: %v", err)
		}
		if got != want {
			t.Fatalf("call %d allowed = %v, want %v", i, got, want)
		}
	}
}

func TestLimiterPerRequestIsolatedAcrossRequests(t *testing.T) {
	l := New(Config{MaxCallsPerRequest: 1})

	ctx1 := l.Begin(context.Background())
	if ok, _ := l.Acquire(ctx1, 1, "s"); !ok {
		t.Fatal("request 1 call 1 should be allowed")
	}
	if ok, _ := l.Acquire(ctx1, 1, "s"); ok {
		t.Fatal("request 1 call 2 should be denied")
	}

	ctx2 := l.Begin(context.Background())
	if ok, _ := l.Acquire(ctx2, 1, "s"); !ok {
		t.Fatal("request 2 call 1 should be allowed (per-request counters are independent)")
	}
}

func TestLimiterPerCompanyWindow(t *testing.T) {
	cache := newFakeCache()
	l := New(Config{Cache: cache, MaxCallsPerCompanyWindow: 3, Window: time.Hour})

	for i, want := range []bool{true, true, true, false} {
		ctx := l.Begin(context.Background())
		got, err := l.Acquire(ctx, 7, "s")
		if err != nil {
			t.Fatalf("Acquire: %v", err)
		}
		if got != want {
			t.Fatalf("company 7 call %d allowed = %v, want %v", i, got, want)
		}
	}

	ctx := l.Begin(context.Background())
	if ok, _ := l.Acquire(ctx, 8, "s"); !ok {
		t.Fatal("company 8 should have its own independent window budget")
	}
}

func TestWindowNoOpsWithoutCache(t *testing.T) {
	l := New(Config{MaxCallsPerCompanyWindow: 1, Window: time.Minute})
	ctx := l.Begin(context.Background())
	if ok, _ := l.Acquire(ctx, 7, "generate"); !ok {
		t.Fatal("call 1 should pass")
	}
	if ok, _ := l.Acquire(ctx, 7, "generate"); !ok {
		t.Fatal("without a cache the per-company window cannot enforce; production must pass a Cache (see cmd/api wiring)")
	}
}

func TestLimiterUnlimitedByDefault(t *testing.T) {
	l := New(Config{})
	ctx := l.Begin(context.Background())
	for i := range 100 {
		if ok, err := l.Acquire(ctx, 1, "s"); err != nil || !ok {
			t.Fatalf("call %d denied with zero limits (want unlimited): ok=%v err=%v", i, ok, err)
		}
	}
}
