package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v5"

	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
	"github.com/my/app/internal/shared/ctxkey"
	"github.com/my/app/internal/transport/http/response"
)

type reportsRepo interface {
	AIAccuracy(ctx context.Context, companyID int64, since time.Time) (reportsmysql.AccuracySnapshot, error)
	AnswerRate(ctx context.Context, companyID int64, since time.Time, threshold float64) (reportsmysql.AnswerRateSnapshot, error)
	KnowledgeGaps(ctx context.Context, companyID int64, limit int) ([]reportsmysql.KnowledgeGapRow, error)
}

type ReportsHandler struct {
	repo reportsRepo
}

func NewReportsHandler(repo reportsRepo) *ReportsHandler {
	return &ReportsHandler{repo: repo}
}

func (h *ReportsHandler) AIAccuracy(c *echo.Context) error {
	cid := ctxkey.MustCompanyID(c.Request().Context())
	since := parseSince(c.QueryParam("since"))
	snap, err := h.repo.AIAccuracy(c.Request().Context(), cid, since)
	if err != nil {
		return response.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, response.OK(map[string]any{
		"total_classified":  snap.TotalClassified,
		"human_confirmed":   snap.HumanConfirmed,
		"problem_type_hits": snap.ProblemTypeHits,
		"subject_hits":      snap.SubjectHits,
		"symptom_hits":      snap.SymptomHits,
		"severity_hits":     snap.SeverityHits,
	}))
}

func (h *ReportsHandler) AnswerRate(c *echo.Context) error {
	cid := ctxkey.MustCompanyID(c.Request().Context())
	since := parseSince(c.QueryParam("since"))
	threshold := parseFloat(c.QueryParam("threshold"), 0.85)
	snap, err := h.repo.AnswerRate(c.Request().Context(), cid, since, threshold)
	if err != nil {
		return response.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, response.OK(map[string]any{
		"total_actions":   snap.TotalActions,
		"high_confidence": snap.HighConf,
		"latency_avg_ms":  snap.LatencyP50Ms,
		"threshold":       threshold,
	}))
}

func (h *ReportsHandler) KnowledgeGaps(c *echo.Context) error {
	cid := ctxkey.MustCompanyID(c.Request().Context())
	limit := parseInt(c.QueryParam("limit"), 20)
	rows, err := h.repo.KnowledgeGaps(c.Request().Context(), cid, limit)
	if err != nil {
		return response.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, response.OK(map[string]any{"gaps": rows}))
}

func parseSince(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if days, err := strconv.Atoi(s); err == nil && days > 0 {
		return time.Now().AddDate(0, 0, -days)
	}
	return time.Time{}
}

func parseFloat(s string, fallback float64) float64 {
	if s == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return fallback
	}
	return f
}

func parseInt(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
