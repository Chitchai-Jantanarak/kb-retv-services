package rag

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

var errCacheMiss = errors.New("cache miss")

func TestPipelineRecomputesDecisionOnCacheHitWhenThresholdChanges(t *testing.T) {
	cache := newMemoryCache()
	threshold := 0.5
	retriever := &recordingRetriever{candidates: []Candidate{{ID: "1", Title: "Fix", Content: "solution", Score: 0.9}}}
	pipeline := NewPipeline(Config{
		Extractor:              DefaultExtractor{},
		Retrievers:             []Retriever{retriever},
		Reranker:               LexicalReranker{},
		Compressor:             Compressor{MaxRunes: 100},
		CRAG:                   fakeCRAG{verdict: VerdictRelevant},
		Generator:              &stubGenerator{draft: "answer"},
		Cache:                  cache,
		CacheTTL:               time.Minute,
		ConfidenceThresholdFor: func(_ context.Context, _ int64) float64 { return threshold },
		Workers:                1,
	})
	q := Query{CompanyID: 1, Text: "help"}

	first, err := pipeline.Run(context.Background(), q)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if first.Decision != DecisionAuto {
		t.Fatalf("first Decision = %q, want auto (threshold 0.5)", first.Decision)
	}

	threshold = 0.95
	second, err := pipeline.Run(context.Background(), q)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if !second.CacheHit {
		t.Fatal("second CacheHit = false, want true")
	}
	if retriever.calls != 1 {
		t.Fatalf("retriever calls = %d, want 1 (second served from cache)", retriever.calls)
	}
	if second.Decision != DecisionDraft {
		t.Fatalf("second Decision = %q, want draft (threshold raised to 0.95 must re-route on cache hit)", second.Decision)
	}
}

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
