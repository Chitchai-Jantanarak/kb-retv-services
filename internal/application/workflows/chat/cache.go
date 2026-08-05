package chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/shared/ctxkey"
)

func cacheKey(companyID int64, principal ctxkey.Principal, req dto.ChatRequest) string {
	parts := make([]string, 0, len(req.Messages)*2+8)
	parts = append(parts,
		strconv.FormatInt(companyID, 10),
		strconv.FormatInt(principal.UserID, 10),
		principal.Role,
		req.Locale,
		permsKey(principal.Perms),
		coverageKey(principal.Coverage),
	)
	for _, m := range req.Messages {
		parts = append(parts, m.Role, m.Content)
		for _, a := range m.Attachments {
			parts = append(parts, a.ID, a.StorageKey)
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "chat:" + hex.EncodeToString(sum[:])
}

func permsKey(perms []string) string {
	sorted := make([]string, len(perms))
	copy(sorted, perms)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

func coverageKey(coverage []int64) string {
	ids := make([]int64, len(coverage))
	copy(ids, coverage)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ",")
}

func (w *Workflow) cachedResponse(ctx context.Context, key string) (dto.ChatResponse, bool) {
	if w.cache == nil {
		return dto.ChatResponse{}, false
	}
	raw, err := w.cache.Get(ctx, key)
	if err != nil || len(raw) == 0 {
		return dto.ChatResponse{}, false
	}
	var resp dto.ChatResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return dto.ChatResponse{}, false
	}
	resp.StageTimingsMS = nil
	resp.Debug = nil
	return resp, true
}

func cacheable(resp dto.ChatResponse) bool {
	return resp.Status == dto.ChatStatusAnswered &&
		resp.Case == nil &&
		len(resp.SearchResults) == 0 &&
		len(resp.Sources) > 0
}

func (w *Workflow) storeResponse(ctx context.Context, key string, resp dto.ChatResponse) {
	if w.cache == nil || strings.TrimSpace(resp.Reply) == "" || !cacheable(resp) {
		return
	}
	resp.StageTimingsMS = nil
	resp.Debug = nil
	raw, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_ = w.cache.Set(ctx, key, raw, w.cacheTTL)
}
