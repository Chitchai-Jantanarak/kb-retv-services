package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

type stubFeedbackRecorder struct {
	got ports.AIActionFeedback
	err error
}

func (s *stubFeedbackRecorder) RecordFeedback(_ context.Context, fb ports.AIActionFeedback) error {
	s.got = fb
	return s.err
}

func newFeedbackContext(t *testing.T, companyID int64, body string) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/reply/feedback", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := ctxkey.WithCompanyID(req.Context(), companyID)
	rec := httptest.NewRecorder()
	c := e.NewContext(req.WithContext(ctx), rec)
	return c, rec
}

func TestFeedbackHandlerRecordsAccepted(t *testing.T) {
	stub := &stubFeedbackRecorder{}
	h := NewFeedbackHandler(stub)
	c, rec := newFeedbackContext(t, 7, `{"ai_action_id":42,"verdict":"accepted"}`)
	if err := h.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if stub.got.CompanyID != 7 || stub.got.ActionID != 42 || stub.got.Verdict != "accepted" || stub.got.Score != 2 {
		t.Fatalf("got = %+v", stub.got)
	}
}

func TestFeedbackHandlerRejectsBadVerdict(t *testing.T) {
	stub := &stubFeedbackRecorder{}
	h := NewFeedbackHandler(stub)
	c, rec := newFeedbackContext(t, 7, `{"ai_action_id":42,"verdict":"nonsense"}`)
	if err := h.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestFeedbackHandlerBubblesRecorderError(t *testing.T) {
	stub := &stubFeedbackRecorder{err: errors.New("not found")}
	h := NewFeedbackHandler(stub)
	c, rec := newFeedbackContext(t, 7, `{"ai_action_id":42,"verdict":"rejected"}`)
	_ = h.Create(c)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200, got %d", rec.Code)
	}
}
