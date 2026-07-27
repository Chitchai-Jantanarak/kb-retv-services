package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/shared/ctxkey"
)

func TestSearchHandlerValidationFailureCases(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"missing query", `{}`, http.StatusBadRequest},
		{"malformed json", `{"query":`, http.StatusBadRequest},
		{"unknown type", `{"query":"printer","types":["web"]}`, http.StatusBadRequest},
		{"bad limit", `{"query":"printer","limit":51}`, http.StatusBadRequest},
		{"valid query", `{"query":"printer offline"}`, http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			h := NewSearchHandler(&capturingSearchWorkflow{})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/search", strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			req = req.WithContext(ctxkey.WithCompanyID(req.Context(), 42))
			c := e.NewContext(req, rec)

			if err := h.Create(c); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestSearchHandlerPassesRequestAndReturnsResponse(t *testing.T) {
	e := echo.New()
	wf := &capturingSearchWorkflow{resp: dto.SearchResponse{
		Query:   "printer offline",
		Results: []dto.SearchResult{{Type: "kb", ID: "chunk:9", Title: "Printer offline"}},
	}}
	h := NewSearchHandler(wf)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/search", strings.NewReader(`{"query":"printer offline"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req = req.WithContext(ctxkey.WithCompanyID(req.Context(), 42))
	c := e.NewContext(req, rec)

	if err := h.Create(c); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if wf.req.Query != "printer offline" {
		t.Fatalf("workflow received query %q, want printer offline", wf.req.Query)
	}
	body := rec.Body.String()
	for _, want := range []string{"chunk:9", "Printer offline"} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q\n%s", want, body)
		}
	}
}

type capturingSearchWorkflow struct {
	req  dto.SearchRequest
	resp dto.SearchResponse
}

func (w *capturingSearchWorkflow) Run(_ context.Context, req dto.SearchRequest) (dto.SearchResponse, error) {
	w.req = req
	return w.resp, nil
}
