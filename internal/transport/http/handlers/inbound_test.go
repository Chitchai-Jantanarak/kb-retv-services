package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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

func TestInboundHandlerAllowsLargerBodyForEmailOnly(t *testing.T) {
	registry, err := omnichannel.NewNormalizerRegistry(omnichannel.LineNormalizer{}, omnichannel.EmailNormalizer{})
	if err != nil {
		t.Fatalf("NewNormalizerRegistry: %v", err)
	}
	padding := strings.Repeat("a", 3<<20)

	t.Run("email between 1MB and 12MB is accepted", func(t *testing.T) {
		workflow := &recordingInboundWorkflow{}
		handler := NewInboundHandler(workflow, registry)
		e := echo.New()
		e.POST("/v1/inbound/:channel", handler.Receive)

		body := `{"message_id":"<m-1@mail>","from":"alice@example.com","subject":"help","body":"` + padding + `"}`
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/inbound/email", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		e.ServeHTTP(rec, req)

		if rec.Code == http.StatusRequestEntityTooLarge {
			t.Fatalf("email body of %d bytes must not be rejected for size: status=%d body=%s", len(body), rec.Code, rec.Body.String())
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if workflow.calls != 1 {
			t.Fatalf("workflow calls = %d, want 1", workflow.calls)
		}
	})

	t.Run("same size to line is rejected", func(t *testing.T) {
		workflow := &recordingInboundWorkflow{}
		handler := NewInboundHandler(workflow, registry)
		e := echo.New()
		e.POST("/v1/inbound/:channel", handler.Receive)

		body := `{"destination":"bot-1","events":[{"source":{"userId":"u"},"message":{"id":"m","type":"text","text":"` + padding + `"}}]}`
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/inbound/line", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		e.ServeHTTP(rec, req)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
		}
		if workflow.calls != 0 {
			t.Fatalf("workflow calls = %d, want 0", workflow.calls)
		}
	})
}

type explodingReader struct{ t *testing.T }

func (r *explodingReader) Read([]byte) (int, error) {
	r.t.Fatal("body must not be read before the missing-signature-header guard runs")
	return 0, io.EOF
}

func TestInboundHandlerRejectsMissingSignatureHeaderBeforeReadingBody(t *testing.T) {
	registry, err := omnichannel.NewNormalizerRegistry(omnichannel.EmailNormalizer{})
	if err != nil {
		t.Fatalf("NewNormalizerRegistry: %v", err)
	}
	workflow := &recordingInboundWorkflow{}
	handler := NewInboundHandler(workflow, registry, WithInboundWebhookSecret("secret"))
	e := echo.New()
	e.POST("/v1/inbound/:channel", handler.Receive)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/inbound/email", &explodingReader{t: t})
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)

	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if workflow.calls != 0 {
		t.Fatalf("workflow calls = %d, want 0", workflow.calls)
	}
}

type errInboundWorkflow struct{ err error }

func (w *errInboundWorkflow) Run(context.Context, omnichannel.Normalized, []byte) (omnichannel.Result, error) {
	return omnichannel.Result{}, w.err
}

func TestInboundHandlerUnknownReceiverReturns404(t *testing.T) {
	registry, err := omnichannel.NewNormalizerRegistry(omnichannel.LineNormalizer{})
	if err != nil {
		t.Fatalf("NewNormalizerRegistry: %v", err)
	}
	body := `{"destination":"bot-1","events":[{"source":{"userId":"u"},"message":{"id":"m","type":"text","text":"hi"}}]}`
	wf := &errInboundWorkflow{err: fmt.Errorf("omnichannel: resolve channel account: %w", omnichannel.ErrAccountNotFound)}
	handler := NewInboundHandler(wf, registry)
	e := echo.New()
	e.POST("/v1/inbound/:channel", handler.Receive)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/inbound/line", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown receiver: status = %d, want 404, body=%s", rec.Code, rec.Body.String())
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
