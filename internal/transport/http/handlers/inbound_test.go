package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/application/workflows/omnichannel"
)

func TestInboundHandlerRequiresValidSignatureWhenSecretConfigured(t *testing.T) {
	registry, err := omnichannel.NewNormalizerRegistry(omnichannel.LineNormalizer{})
	if err != nil {
		t.Fatalf("NewNormalizerRegistry: %v", err)
	}
	body := `{"destination":"bot-1","events":[{"source":{"userId":"user-1"},"message":{"id":"msg-1","type":"text","text":"hello"}}]}`

	cases := []struct {
		name       string
		signature  string
		wantStatus int
		wantCalls  int
	}{
		{name: "missing", signature: "", wantStatus: http.StatusUnauthorized},
		{name: "invalid", signature: "bad", wantStatus: http.StatusUnauthorized},
		{name: "valid", signature: signInboundTestBody("secret", []byte(body)), wantStatus: http.StatusOK, wantCalls: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workflow := &recordingInboundWorkflow{}
			handler := NewInboundHandler(workflow, registry, WithInboundWebhookSecret("secret"))
			e := echo.New()
			e.POST("/v1/inbound/:channel", handler.Receive)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/inbound/line", strings.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			if tc.signature != "" {
				req.Header.Set("X-AI-Signature", tc.signature)
			}

			e.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if workflow.calls != tc.wantCalls {
				t.Fatalf("workflow calls = %d, want %d", workflow.calls, tc.wantCalls)
			}
		})
	}
}

func TestInboundHandlerRejectsOversizedBody(t *testing.T) {
	registry, err := omnichannel.NewNormalizerRegistry(omnichannel.LineNormalizer{})
	if err != nil {
		t.Fatalf("NewNormalizerRegistry: %v", err)
	}
	body := []byte(`{"destination":"bot-1","events":[]}`)
	workflow := &recordingInboundWorkflow{}
	handler := NewInboundHandler(
		workflow,
		registry,
		WithInboundWebhookSecret("secret"),
		WithInboundMaxBodyBytes(int64(len(body)-1)),
	)
	e := echo.New()
	e.POST("/v1/inbound/:channel", handler.Receive)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/inbound/line", strings.NewReader(string(body)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("X-AI-Signature", signInboundTestBody("secret", body))

	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
	if workflow.calls != 0 {
		t.Fatalf("workflow calls = %d, want 0", workflow.calls)
	}
}

type recordingInboundWorkflow struct {
	calls int
}

func (w *recordingInboundWorkflow) Run(context.Context, omnichannel.Normalized, []byte) (omnichannel.Result, error) {
	w.calls++
	return omnichannel.Result{CompanyID: 1, ConversationID: 2, MessageID: 3, TicketEnqueued: true}, nil
}

func signInboundTestBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
