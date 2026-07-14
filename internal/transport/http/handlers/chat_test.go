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

func TestChatHandlerValidationFailureCases(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"missing messages", `{"locale":"en"}`, http.StatusBadRequest},
		{"empty content", `{"messages":[{"role":"user","content":"   "}]}`, http.StatusBadRequest},
		{"invalid role", `{"messages":[{"role":"system","content":"hi"}]}`, http.StatusBadRequest},
		{"malformed json", `{"messages":`, http.StatusBadRequest},
		{"valid user message", `{"messages":[{"role":"user","content":"hello"}]}`, http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			h := NewChatHandler(&capturingChatWorkflow{})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(tc.body))
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

func TestChatHandlerPassesRequestAndReturnsResponse(t *testing.T) {
	e := echo.New()
	wf := &capturingChatWorkflow{resp: dto.ChatResponse{
		Reply:         "hi",
		SearchResults: []dto.ChatCaseResult{{ID: 9, Title: "Printer offline", Status: "waiting"}},
	}}
	h := NewChatHandler(wf)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat",
		strings.NewReader(`{"messages":[{"role":"user","content":"hello"}],"locale":"en"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req = req.WithContext(ctxkey.WithCompanyID(req.Context(), 42))
	c := e.NewContext(req, rec)

	if err := h.Create(c); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if wf.req.LastUserMessage() != "hello" {
		t.Fatalf("workflow received last user %q, want hello", wf.req.LastUserMessage())
	}
	body := rec.Body.String()
	for _, want := range []string{"hi", "Printer offline", "search_results"} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q\n%s", want, body)
		}
	}
}

type capturingChatWorkflow struct {
	req  dto.ChatRequest
	resp dto.ChatResponse
}

func (w *capturingChatWorkflow) Run(_ context.Context, req dto.ChatRequest) (dto.ChatResponse, error) {
	w.req = req
	return w.resp, nil
}
