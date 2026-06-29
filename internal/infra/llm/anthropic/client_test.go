package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/llmerr"
)

func TestGenerateReturnsTypedProviderErrorOn503(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`))
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	_, err = c.Generate(context.Background(), ports.Prompt{User: "x"})
	var pe *llmerr.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v (%T), want *llmerr.ProviderError", err, err)
	}
	if pe.Status != http.StatusServiceUnavailable {
		t.Fatalf("Status = %d, want 503", pe.Status)
	}
	if d, ok := llmerr.Retryable(err); !ok || d != 2*time.Second {
		t.Fatalf("Retryable = (%v, %v), want (2s, true)", d, ok)
	}
}

func TestNewRejectsEmptyAPIKey(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("New(Config{}) err = nil, want api key error")
	}
}

func TestGenerateSendsExpectedRequest(t *testing.T) {
	var captured struct {
		apiKey  string
		version string
		body    messagesRequest
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.apiKey = r.Header.Get("x-api-key")
		captured.version = r.Header.Get("anthropic-version")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured.body)
		_ = json.NewEncoder(w).Encode(messagesResponse{
			Content: []messageContent{{Type: "text", Text: "hi"}},
			Usage:   messagesUsage{InputTokens: 5, OutputTokens: 2},
		})
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "sk-ant-x", BaseURL: srv.URL, Model: "claude-haiku-4-5-20251001"})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	out, err := c.Generate(context.Background(), ports.Prompt{System: "be terse", User: "hello", MaxToks: 50})
	if err != nil {
		t.Fatalf("Generate() err = %v", err)
	}
	if out.Text != "hi" {
		t.Fatalf("Text = %q, want hi", out.Text)
	}
	if out.Usage.Total != 7 {
		t.Fatalf("Total = %d, want 7", out.Usage.Total)
	}
	if captured.apiKey != "sk-ant-x" {
		t.Fatalf("x-api-key = %s", captured.apiKey)
	}
	if captured.version == "" {
		t.Fatal("anthropic-version header missing")
	}
	if captured.body.System != "be terse" {
		t.Fatalf("system = %s, want 'be terse'", captured.body.System)
	}
	if captured.body.MaxTokens != 50 {
		t.Fatalf("max_tokens = %d, want 50", captured.body.MaxTokens)
	}
}

func TestGenerateJSONAppendsJSONOnlySystemInstruction(t *testing.T) {
	var captured messagesRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(messagesResponse{
			Content: []messageContent{{Type: "text", Text: `{"ok":true}`}},
		})
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if _, err := c.GenerateJSON(context.Background(), ports.Prompt{System: "be precise", User: "json please"}); err != nil {
		t.Fatalf("GenerateJSON() err = %v", err)
	}
	if !strings.Contains(captured.System, "be precise") || !strings.Contains(captured.System, "JSON") {
		t.Fatalf("system did not combine both prompts: %q", captured.System)
	}
}

func TestGenerateFailureCases(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantSub string
	}{
		{name: "http_500", status: 500, body: "boom", wantSub: "status 500"},
		{name: "api_error", status: 200, body: `{"error":{"type":"overloaded","message":"upstream overloaded"}}`, wantSub: "upstream overloaded"},
		{name: "empty_content", status: 200, body: `{"content":[]}`, wantSub: "empty content"},
		{name: "no_text_blocks", status: 200, body: `{"content":[{"type":"tool_use","text":""}]}`, wantSub: "no text blocks"},
		{name: "decode_error", status: 200, body: "not json", wantSub: "decode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c, err := New(Config{APIKey: "k", BaseURL: srv.URL})
			if err != nil {
				t.Fatalf("New() err = %v", err)
			}
			_, err = c.Generate(context.Background(), ports.Prompt{User: "x"})
			if err == nil {
				t.Fatalf("Generate() err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}
