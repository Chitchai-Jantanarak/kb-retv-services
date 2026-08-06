package chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/skeleton"
)

func summaryComposeMode(r skeleton.Response, req dto.ChatRequest) bool {
	if req.Mode == dto.ChatModeVoice {
		return false
	}
	return r.ComposeMode != "template"
}

var summaryCodeRe = regexp.MustCompile(`(?i)\bREP-?\d+\b`)

func (w *Workflow) composeToolReply(ctx context.Context, locale, question string, companyID int64, r skeleton.Response) (string, bool) {
	if r.Table == nil || w.resolve == nil || len(r.Table.Rows) == 0 {
		return "", false
	}

	rowsText := strings.Join(r.Lines, "\n")
	key := toolSummaryCacheKey(r, rowsText, locale)
	if w.cache != nil {
		if raw, err := w.cache.Get(ctx, key); err == nil && len(raw) > 0 {
			return string(raw), true
		}
	}

	summary, ok := w.smallModelCall(ctx, w.summaryTmpl, map[string]string{
		"language": promptLanguage(locale),
		"question": question,
		"count":    strconv.Itoa(len(r.Table.Rows)),
		"rows":     rowsText,
	}, companyID)
	if !ok || !summaryCodesGrounded(summary, rowsText) {
		return "", false
	}

	if w.cache != nil {
		_ = w.cache.Set(ctx, key, []byte(summary), w.cacheTTL)
	}
	return summary, true
}

func summaryCodesGrounded(summary, rowsText string) bool {
	rowCodes := make(map[string]bool)
	for _, code := range summaryCodeRe.FindAllString(rowsText, -1) {
		rowCodes[normalizeCode(code)] = true
	}
	for _, code := range summaryCodeRe.FindAllString(summary, -1) {
		if !rowCodes[normalizeCode(code)] {
			return false
		}
	}
	return true
}

func normalizeCode(code string) string {
	digits := strings.TrimLeft(strings.TrimPrefix(strings.ToUpper(code), "REP"), "-")
	return "REP-" + digits
}

func toolSummaryCacheKey(r skeleton.Response, rowsText, locale string) string {
	keys := make([]string, 0, len(r.Params))
	for k := range r.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{"toolsum", r.ToolID, locale, rowsText}
	for _, k := range keys {
		parts = append(parts, k+"="+r.Params[k])
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "toolsum:" + hex.EncodeToString(sum[:])
}
