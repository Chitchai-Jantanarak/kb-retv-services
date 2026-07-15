package chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/my/app/internal/application/dto"
)

func cacheKey(companyID int64, req dto.ChatRequest) string {
	parts := make([]string, 0, len(req.Messages)*2+2)
	parts = append(parts, strconv.FormatInt(companyID, 10), req.Locale)
	for _, m := range req.Messages {
		parts = append(parts, m.Role, m.Content)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "chat:" + hex.EncodeToString(sum[:])
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
	return resp, true
}

func sanitizeForCache(resp dto.ChatResponse) dto.ChatResponse {
	resp.StageTimingsMS = nil
	resp.SearchResults = nil
	resp.Case = nil
	return resp
}

func (w *Workflow) storeResponse(ctx context.Context, key string, resp dto.ChatResponse) {
	if w.cache == nil || strings.TrimSpace(resp.Reply) == "" {
		return
	}
	raw, err := json.Marshal(sanitizeForCache(resp))
	if err != nil {
		return
	}
	_ = w.cache.Set(ctx, key, raw, w.cacheTTL)
}
