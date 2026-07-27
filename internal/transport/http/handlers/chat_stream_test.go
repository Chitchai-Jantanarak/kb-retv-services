package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/shared/ctxkey"
)

var errChatStreamWorkflowFailed = errors.New("workflow: failed")

type capturingChatStreamWorkflow struct {
	req    dto.ChatRequest
	events []dto.ChatStreamEvent
	err    error
}

func (w *capturingChatStreamWorkflow) RunStream(_ context.Context, req dto.ChatRequest, emit func(dto.ChatStreamEvent) error) error {
	w.req = req
	for _, e := range w.events {
		if err := emit(e); err != nil {
			return err
		}
	}
	return w.err
}

func TestChatStreamHandlerValidationFailureBeforeHeaders(t *testing.T) {
	e := echo.New()
	h := NewChatStreamHandler(&capturingChatStreamWorkflow{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/stream", strings.NewReader(`{"locale":"en"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req = req.WithContext(ctxkey.WithCompanyID(req.Context(), 42))
	c := e.NewContext(req, rec)

	if err := h.Create(c); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (body=%s)", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if ct := rec.Header().Get(echo.HeaderContentType); strings.Contains(ct, "event-stream") {
		t.Fatalf("Content-Type = %q, want JSON error not SSE", ct)
	}
}

func TestChatStreamHandlerWritesSSEEventsForDeltasAndDone(t *testing.T) {
	e := echo.New()
	wf := &capturingChatStreamWorkflow{events: []dto.ChatStreamEvent{
		{Type: dto.ChatStreamEventDelta, Text: "Hel"},
		{Type: dto.ChatStreamEventDelta, Text: "lo"},
		{Type: dto.ChatStreamEventDone, Status: dto.ChatStatusAnswered},
	}}
	h := NewChatStreamHandler(wf)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/stream",
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
	if ct := rec.Header().Get(echo.HeaderContentType); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", rec.Header().Get("Cache-Control"))
	}

	body := rec.Body.String()
	wantLines := []string{
		"event: delta", `data: {"text":"Hel"}`,
		"event: delta", `data: {"text":"lo"}`,
		"event: done", `"status":"answered"`,
	}
	for _, want := range wantLines {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q\nbody=%s", want, body)
		}
	}
	if wf.req.LastUserMessage() != "hello" {
		t.Fatalf("workflow received last user %q, want hello", wf.req.LastUserMessage())
	}
}

func TestChatStreamHandlerEmitsErrorEventWhenWorkflowFails(t *testing.T) {
	e := echo.New()
	wf := &capturingChatStreamWorkflow{err: errChatStreamWorkflowFailed}
	h := NewChatStreamHandler(wf)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/stream",
		strings.NewReader(`{"messages":[{"role":"user","content":"hello"}],"locale":"en"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req = req.WithContext(ctxkey.WithCompanyID(req.Context(), 42))
	c := e.NewContext(req, rec)

	if err := h.Create(c); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !strings.Contains(rec.Body.String(), "event: error") {
		t.Fatalf("body missing error event: %s", rec.Body.String())
	}
}
