package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
	"github.com/my/app/internal/shared/ctxkey"
)

type stubReports struct {
	accSnap  reportsmysql.AccuracySnapshot
	accErr   error
	rateSnap reportsmysql.AnswerRateSnapshot
	rateErr  error
	gaps     []reportsmysql.KnowledgeGapRow
	gapsErr  error
	lastCID  int64
}

func (s *stubReports) AIAccuracy(_ context.Context, companyID int64, _ time.Time) (reportsmysql.AccuracySnapshot, error) {
	s.lastCID = companyID
	return s.accSnap, s.accErr
}

func (s *stubReports) AnswerRate(_ context.Context, companyID int64, _ time.Time, _ float64) (reportsmysql.AnswerRateSnapshot, error) {
	s.lastCID = companyID
	return s.rateSnap, s.rateErr
}

func (s *stubReports) KnowledgeGaps(_ context.Context, companyID int64, _ int) ([]reportsmysql.KnowledgeGapRow, error) {
	s.lastCID = companyID
	return s.gaps, s.gapsErr
}

func newRequestWithCompany(method, target string) (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(""))
	rec := httptest.NewRecorder()
	ctx := ctxkey.WithCompanyID(req.Context(), 7)
	req = req.WithContext(ctx)
	c := e.NewContext(req, rec)
	return c, rec
}

func TestAIAccuracyHandlerReturnsSnapshot(t *testing.T) {
	stub := &stubReports{accSnap: reportsmysql.AccuracySnapshot{TotalClassified: 10, ProblemTypeHits: 8}}
	h := NewReportsHandler(stub)
	c, rec := newRequestWithCompany(http.MethodGet, "/v1/reports/ai-accuracy")
	if err := h.AIAccuracy(c); err != nil {
		t.Fatalf("AIAccuracy: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if stub.lastCID != 7 {
		t.Fatalf("company_id = %d, want 7", stub.lastCID)
	}
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v", err)
	}
	if payload.Data["total_classified"].(float64) != 10 {
		t.Fatalf("total_classified = %v", payload.Data["total_classified"])
	}
}

func TestAnswerRateHandlerSurfacesRepoError(t *testing.T) {
	stub := &stubReports{rateErr: errors.New("db down")}
	h := NewReportsHandler(stub)
	c, rec := newRequestWithCompany(http.MethodGet, "/v1/reports/answer-rate")
	_ = h.AnswerRate(c)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestKnowledgeGapsHandlerReturnsList(t *testing.T) {
	stub := &stubReports{gaps: []reportsmysql.KnowledgeGapRow{
		{ID: 1, QueryText: "printer dead", OccurrenceCount: 12},
	}}
	h := NewReportsHandler(stub)
	c, rec := newRequestWithCompany(http.MethodGet, "/v1/reports/knowledge-gaps?limit=5")
	if err := h.KnowledgeGaps(c); err != nil {
		t.Fatalf("KnowledgeGaps: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload struct {
		Data struct {
			Gaps []map[string]any `json:"gaps"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(payload.Data.Gaps) != 1 || payload.Data.Gaps[0]["query_text"] != "printer dead" {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
