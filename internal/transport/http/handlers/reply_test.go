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

func TestReplyHandlerValidationFailureCases(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"missing message", `{"customer_id":"c1"}`, http.StatusBadRequest},
		{"empty message", `{"customer_id":"c1","message":"   "}`, http.StatusBadRequest},
		{"invalid channel", `{"customer_id":"c1","message":"hi","channel":"fax"}`, http.StatusBadRequest},
		{"base64 data url attachment", `{"customer_id":"c1","message":"hi","attachments":[{"url":"data:image/png;base64,AAAA"}]}`, http.StatusBadRequest},
		{"malformed json", `{"customer_id":`, http.StatusBadRequest},
		{"query alias accepted", `{"query":"hello"}`, http.StatusOK},
		{"in_app channel accepted", `{"query":"hello","channel":"in_app"}`, http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			h := NewReplyHandler(&capturingWorkflow{})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/reply", strings.NewReader(tc.body))
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

func TestReplyHandlerPassesCompanyIDViaContext(t *testing.T) {
	e := echo.New()
	workflow := &capturingWorkflow{}
	h := NewReplyHandler(workflow)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/reply", strings.NewReader(`{"customer_id":"c1","message":"hello"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req = req.WithContext(ctxkey.WithCompanyID(req.Context(), 42))
	c := e.NewContext(req, rec)

	if err := h.Create(c); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

type capturingWorkflow struct {
	req dto.ReplyRequest
}

func (w *capturingWorkflow) Run(ctx context.Context, req dto.ReplyRequest) (dto.ReplyResponse, error) {
	w.req = req
	return dto.ReplyResponse{Intent: "ok"}, nil
}
