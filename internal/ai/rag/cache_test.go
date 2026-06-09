package rag

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

var errCacheMiss = errors.New("cache miss")

type memoryCache struct {
	mu    sync.Mutex
	items map[string][]byte
}

func newMemoryCache() *memoryCache {
	return &memoryCache{items: make(map[string][]byte)}
}

func (c *memoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.items[key]
	if !ok {
		return nil, errCacheMiss
	}
	copied := make([]byte, len(value))
	copy(copied, value)
	return copied, nil
}

func (c *memoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := make([]byte, len(value))
	copy(copied, value)
	c.items[key] = copied
	return nil
}

func (c *memoryCache) Delete(ctx context.Context, keys ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		delete(c.items, key)
	}
	return nil
}

func (c *memoryCache) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[key]; ok {
		return false, nil
	}
	copied := make([]byte, len(value))
	copy(copied, value)
	c.items[key] = copied
	return true, nil
}

func TestCacheKeyIncludesMode(t *testing.T) {
	base := Query{CompanyID: 3, Text: "PUDU robot offline"}
	fast := base
	fast.Mode = ModeFastDraft
	full := base
	full.Mode = ModeFullReview

	if cacheKey(base) == cacheKey(fast) {
		t.Fatal("default and fast_draft cache keys should differ")
	}
	if cacheKey(fast) == cacheKey(full) {
		t.Fatal("fast_draft and full_review cache keys should differ")
	}
}

func TestCacheKeyForPlanIncludesGraphAnchors(t *testing.T) {
	query := Query{CompanyID: 3, Text: "PUDU tray jam"}
	basePlan := QueryPlan{CanonicalText: "pudu tray jam"}
	graphPlan := QueryPlan{
		CanonicalText: "pudu tray jam",
		GraphAnchors:  []GraphAnchor{{Kind: GraphAnchorSymptom, ID: 44}},
	}

	if cacheKeyForPlan(query, basePlan) == cacheKeyForPlan(query, graphPlan) {
		t.Fatal("cache keys should differ when graph anchors differ")
	}
}
