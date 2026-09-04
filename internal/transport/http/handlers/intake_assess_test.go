package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/application/workflows/omnichannel"
	"github.com/my/app/internal/shared/ctxkey"
)

type stubManualIntakeAssessor struct {
	companyID      int64
	conversationID int64
	messageID      int64
	err            error
}

func (s *stubManualIntakeAssessor) EnqueueDraft(_ context.Context, companyID, conversationID int64) (int64, error) {
	s.companyID = companyID
	s.conversationID = conversationID
	return s.messageID, s.err
}

func newIntakeAssessContext(t *testing.T, companyID int64, body string) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/intake/assess", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctxkey.WithCompanyID(req.Context(), companyID))
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestIntakeAssessHandlerQueuesTenantDraft(t *testing.T) {
	stub := &stubManualIntakeAssessor{messageID: 88}
	handler := NewIntakeAssessHandler(stub)
	c, rec := newIntakeAssessContext(t, 7, `{"conversation_id":42}`)

	if err := handler.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if stub.companyID != 7 || stub.conversationID != 42 {
		t.Fatalf("queued company/conversation = %d/%d, want 7/42", stub.companyID, stub.conversationID)
	}
	if !strings.Contains(rec.Body.String(), `"message_id":88`) {
		t.Fatalf("body = %s, want message_id 88", rec.Body.String())
	}
}

func TestIntakeAssessHandlerMapsMissingDraftToNotFound(t *testing.T) {
	stub := &stubManualIntakeAssessor{err: omnichannel.ErrDraftNotFound}
	handler := NewIntakeAssessHandler(stub)
	c, rec := newIntakeAssessContext(t, 7, `{"conversation_id":42}`)

	if err := handler.Create(c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}
