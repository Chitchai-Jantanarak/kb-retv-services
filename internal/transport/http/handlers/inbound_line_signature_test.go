package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/application/workflows/omnichannel"
)

func signLineTestBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestInboundHandlerUsesChannelVerifierForLine(t *testing.T) {
	registry, err := omnichannel.NewNormalizerRegistry(omnichannel.LineNormalizer{})
	if err != nil {
		t.Fatalf("NewNormalizerRegistry: %v", err)
	}
	body := `{"destination":"bot-1","events":[{"source":{"userId":"user-1"},"message":{"id":"msg-1","type":"text","text":"hello"}}]}`

	cases := []struct {
		name       string
		header     string
		value      string
		wantStatus int
		wantCalls  int
	}{
		{name: "valid line signature", header: "X-Line-Signature", value: signLineTestBody("chan-secret", []byte(body)), wantStatus: http.StatusOK, wantCalls: 1},
		{name: "invalid line signature", header: "X-Line-Signature", value: signLineTestBody("wrong-secret", []byte(body)), wantStatus: http.StatusUnauthorized},
		{name: "garbage signature", header: "X-Line-Signature", value: "not-base64!!", wantStatus: http.StatusUnauthorized},
		{name: "missing header", wantStatus: http.StatusUnauthorized},
		{name: "generic header ignored for line", header: "X-AI-Signature", value: signInboundTestBody("secret", []byte(body)), wantStatus: http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workflow := &recordingInboundWorkflow{}
			handler := NewInboundHandler(workflow, registry,
				WithInboundWebhookSecret("secret"),
				WithChannelVerifier("line", LineSignatureVerifier("chan-secret")),
			)
			e := echo.New()
			e.POST("/v1/inbound/:channel", handler.Receive)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/inbound/line", strings.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			if tc.header != "" {
				req.Header.Set(tc.header, tc.value)
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

func TestInboundHandlerFallsBackToDefaultVerifierForOtherChannels(t *testing.T) {
	registry, err := omnichannel.NewNormalizerRegistry(omnichannel.LineNormalizer{})
	if err != nil {
		t.Fatalf("NewNormalizerRegistry: %v", err)
	}
	body := `{"destination":"bot-1","events":[{"source":{"userId":"user-1"},"message":{"id":"msg-1","type":"text","text":"hello"}}]}`

	workflow := &recordingInboundWorkflow{}
	handler := NewInboundHandler(workflow, registry,
		WithInboundWebhookSecret("secret"),
		WithChannelVerifier("other", LineSignatureVerifier("chan-secret")),
	)
	e := echo.New()
	e.POST("/v1/inbound/:channel", handler.Receive)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/inbound/line", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("X-AI-Signature", signInboundTestBody("secret", []byte(body)))

	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if workflow.calls != 1 {
		t.Fatalf("workflow calls = %d, want 1", workflow.calls)
	}
}

func TestLineSignatureVerifierEmptySecretRejects(t *testing.T) {
	body := []byte(`{"events":[]}`)
	verify := LineSignatureVerifier("")
	req := httptest.NewRequest(http.MethodPost, "/v1/inbound/line", strings.NewReader(string(body)))
	req.Header.Set("X-Line-Signature", signLineTestBody("", body))
	if err := verify(req, body); err == nil {
		t.Fatal("expected empty-secret verifier to reject, got nil error")
	}
}

func identityDecrypt(s string) (string, bool) { return s, true }

func TestLineAccountSignatureVerifierUsesPerAccountSecret(t *testing.T) {
	body := []byte(`{"destination":"bot-1","events":[]}`)
	lookup := func(ctx context.Context, destination string) (string, error) {
		if destination != "bot-1" {
			t.Fatalf("unexpected destination %q", destination)
		}
		return "account-secret", nil
	}
	verify := LineAccountSignatureVerifier(lookup, identityDecrypt, "env-secret")

	req := httptest.NewRequest(http.MethodPost, "/v1/inbound/line", strings.NewReader(string(body)))
	req.Header.Set("X-Line-Signature", signLineTestBody("account-secret", body))
	if err := verify(req, body); err != nil {
		t.Fatalf("expected account-secret signature to pass, got %v", err)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/v1/inbound/line", strings.NewReader(string(body)))
	req2.Header.Set("X-Line-Signature", signLineTestBody("env-secret", body))
	if err := verify(req2, body); err == nil {
		t.Fatal("expected env-secret signature to fail when an account secret exists")
	}
}

func TestLineAccountSignatureVerifierFallsBackToEnvSecret(t *testing.T) {
	body := []byte(`{"destination":"bot-2","events":[]}`)
	lookup := func(ctx context.Context, destination string) (string, error) {
		return "", nil
	}
	verify := LineAccountSignatureVerifier(lookup, identityDecrypt, "env-secret")

	req := httptest.NewRequest(http.MethodPost, "/v1/inbound/line", strings.NewReader(string(body)))
	req.Header.Set("X-Line-Signature", signLineTestBody("env-secret", body))
	if err := verify(req, body); err != nil {
		t.Fatalf("expected env-secret fallback to pass, got %v", err)
	}
}

func TestLineAccountSignatureVerifierRejectsWhenNoSecretAvailable(t *testing.T) {
	body := []byte(`{"destination":"bot-3","events":[]}`)
	lookup := func(ctx context.Context, destination string) (string, error) {
		return "", nil
	}
	verify := LineAccountSignatureVerifier(lookup, identityDecrypt, "")

	req := httptest.NewRequest(http.MethodPost, "/v1/inbound/line", strings.NewReader(string(body)))
	req.Header.Set("X-Line-Signature", signLineTestBody("anything", body))
	if err := verify(req, body); err == nil {
		t.Fatal("expected verifier to reject when neither account nor env secret is configured")
	}
}
