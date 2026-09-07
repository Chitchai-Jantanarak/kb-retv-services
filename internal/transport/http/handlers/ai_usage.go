package handlers

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

	aimysql "github.com/my/app/internal/repositories/ai/mysql"
	"github.com/my/app/internal/shared/ctxkey"
	"github.com/my/app/internal/transport/http/response"
)

type aiUsageRepo interface {
	Usage(ctx context.Context, companyID int64, days int) (aimysql.UsageSnapshot, error)
}

type AIUsageHandler struct {
	repo aiUsageRepo
}

func NewAIUsageHandler(repo aiUsageRepo) *AIUsageHandler {
	return &AIUsageHandler{repo: repo}
}

func (h *AIUsageHandler) Usage(c *echo.Context) error {
	cid := ctxkey.MustCompanyID(c.Request().Context())
	days := clampDays(parseInt(c.QueryParam("days"), 7))

	snap, err := h.repo.Usage(c.Request().Context(), cid, days)
	if err != nil {
		return response.WriteError(c, err)
	}

	return c.JSON(http.StatusOK, response.OK(map[string]any{
		"days":         days,
		"totals":       usageTotalsJSON(snap.Totals),
		"today":        usageTodayJSON(snap.Today),
		"daily":        usageDailyJSON(snap.Daily),
		"by_model":     usageByModelJSON(snap.ByModel),
		"models_daily": usageModelsDailyJSON(snap.ModelsDaily),
		"recent":       usageRecentJSON(snap.Recent),
	}))
}

func clampDays(days int) int {
	if days < 1 {
		return 7
	}
	if days > 30 {
		return 30
	}
	return days
}

func usageTotalsJSON(t aimysql.UsageTotals) map[string]any {
	return map[string]any{
		"requests":       t.Requests,
		"errors":         t.Errors,
		"input_tokens":   t.InputTokens,
		"output_tokens":  t.OutputTokens,
		"latency_avg_ms": t.LatencyAvgMs,
		"latency_p50_ms": t.LatencyP50Ms,
		"latency_p95_ms": t.LatencyP95Ms,
	}
}

func usageTodayJSON(t aimysql.UsageToday) map[string]any {
	return map[string]any{
		"requests":      t.Requests,
		"errors":        t.Errors,
		"input_tokens":  t.InputTokens,
		"output_tokens": t.OutputTokens,
	}
}

func usageDailyJSON(days []aimysql.UsageDay) []map[string]any {
	out := make([]map[string]any, 0, len(days))
	for _, d := range days {
		byVendor := d.ByVendor
		if byVendor == nil {
			byVendor = map[string]int{}
		}
		byStatus := d.ByStatus
		if byStatus == nil {
			byStatus = map[string]int{}
		}
		out = append(out, map[string]any{
			"date":          d.Date,
			"requests":      d.Requests,
			"errors":        d.Errors,
			"input_tokens":  d.InputTokens,
			"output_tokens": d.OutputTokens,
			"by_vendor":     byVendor,
			"by_status":     byStatus,
		})
	}
	return out
}

func usageModelsDailyJSON(models []aimysql.UsageModelDaily) []map[string]any {
	out := make([]map[string]any, 0, len(models))
	for _, m := range models {
		daily := make([]map[string]any, 0, len(m.Daily))
		for _, d := range m.Daily {
			daily = append(daily, map[string]any{
				"date":          d.Date,
				"requests":      d.Requests,
				"errors":        d.Errors,
				"input_tokens":  d.InputTokens,
				"output_tokens": d.OutputTokens,
			})
		}
		out = append(out, map[string]any{
			"vendor": m.Vendor,
			"model":  m.Model,
			"daily":  daily,
		})
	}
	return out
}

func usageByModelJSON(models []aimysql.UsageModel) []map[string]any {
	out := make([]map[string]any, 0, len(models))
	for _, m := range models {
		out = append(out, map[string]any{
			"vendor":         m.Vendor,
			"model":          m.Model,
			"requests":       m.Requests,
			"errors":         m.Errors,
			"input_tokens":   m.InputTokens,
			"output_tokens":  m.OutputTokens,
			"latency_avg_ms": m.LatencyAvgMs,
		})
	}
	return out
}

func usageRecentJSON(rows []aimysql.UsageRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"ts":            r.Timestamp,
			"vendor":        r.Vendor,
			"model":         r.Model,
			"op":            r.Op,
			"status":        r.Status,
			"http_status":   r.HTTPStatus,
			"latency_ms":    r.LatencyMs,
			"input_tokens":  r.InputTokens,
			"output_tokens": r.OutputTokens,
			"error":         r.Error,
		})
	}
	return out
}
