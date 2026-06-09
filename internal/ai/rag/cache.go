package rag

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/pkg/concurrency"
)

var bufferPool = concurrency.NewObjectPool(func() *bytes.Buffer {
	return bytes.NewBuffer(make([]byte, 0, 4096))
})

func cacheKey(query Query) string {
	return cacheKeyForText(query, canonicalQueryText(query.Text))
}

func cacheKeyForPlan(query Query, plan QueryPlan) string {
	text := plan.CanonicalText
	if text == "" {
		text = canonicalQueryText(query.Text)
	}
	return cacheKeyForText(query, text, planCacheFingerprint(plan))
}

func cacheKeyForText(query Query, text string, discriminators ...string) string {
	parts := []string{
		strconv.FormatInt(query.CompanyID, 10),
		text,
		strings.ToLower(strings.TrimSpace(query.Mode)),
	}
	for _, discriminator := range discriminators {
		discriminator = strings.TrimSpace(discriminator)
		if discriminator != "" {
			parts = append(parts, discriminator)
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "rag:" + hex.EncodeToString(sum[:])
}

func planCacheFingerprint(plan QueryPlan) string {
	parts := make([]string, 0, 2)
	if queries := plannedQueryFingerprint(plan.RetrievalQueries); queries != "" {
		parts = append(parts, "queries="+queries)
	}
	if anchors := graphAnchorFingerprint(plan.GraphAnchors); anchors != "" {
		parts = append(parts, "graph="+anchors)
	}
	return strings.Join(parts, "\x00")
}

func plannedQueryFingerprint(queries []PlannedQuery) string {
	if len(queries) == 0 {
		return ""
	}
	parts := make([]string, 0, len(queries))
	for _, query := range queries {
		text := canonicalQueryText(query.Text)
		if text == "" {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(query.Kind))
		if kind == "" {
			kind = PlannedQueryOriginal
		}
		parts = append(parts, kind+":"+text)
	}
	return strings.Join(parts, "|")
}

func graphAnchorFingerprint(anchors []GraphAnchor) string {
	if len(anchors) == 0 {
		return ""
	}
	parts := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		if anchor.ID <= 0 || strings.TrimSpace(anchor.Kind) == "" {
			continue
		}
		parts = append(parts, strings.ToLower(strings.TrimSpace(anchor.Kind))+":"+strconv.FormatInt(anchor.ID, 10))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func getCached(ctx context.Context, cache ports.Cache, key string) (Result, bool, error) {
	if cache == nil {
		return Result{}, false, nil
	}
	body, err := cache.Get(ctx, key)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Result{}, false, err
		}
		return Result{}, false, nil
	}
	var result Result
	if err := json.Unmarshal(body, &result); err != nil {
		return Result{}, false, err
	}
	result.CacheHit = true
	return result, true, nil
}

func setCached(ctx context.Context, cache ports.Cache, key string, result Result, ttl time.Duration) error {
	if cache == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = time.Minute
	}

	buf := bufferPool.Get()
	buf.Reset()
	defer bufferPool.Put(buf)

	if err := json.NewEncoder(buf).Encode(result); err != nil {
		return err
	}
	body := make([]byte, buf.Len())
	copy(body, buf.Bytes())
	return cache.Set(ctx, key, body, ttl)
}
