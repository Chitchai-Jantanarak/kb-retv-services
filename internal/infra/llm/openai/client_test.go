package openai

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
		_, _ = w.Write([]byte(`{"error":{"message":"overloaded"}}`))
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
		t.Fatal("New(Config{}) error = nil, want api key error")
	}
}

func TestGenerateSendsExpectedRequest(t *testing.T) {
	var captured struct {
		method  string
		path    string
		auth    string
		referer string
		title   string
		body    chatRequest
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.auth = r.Header.Get("Authorization")
		captured.referer = r.Header.Get("HTTP-Referer")
		captured.title = r.Header.Get("X-Title")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured.body)

		_ = json.NewEncoder(w).Encode(chatResponse{
			Choices: []chatChoice{{Message: chatMessage{Role: "assistant", Content: "hi"}}},
			Usage:   chatUsage{PromptTokens: 3, CompletionTokens: 1, TotalTokens: 4},
			Model:   "gpt-4o-mini",
		})
	}))
	defer srv.Close()

	c, err := New(Config{
		APIKey:      "sk-test",
		BaseURL:     srv.URL,
		Model:       "gpt-4o-mini",
		ReferrerURL: "https://example.com",
		Title:       "smartjob",
	})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	out, err := c.Generate(context.Background(), ports.Prompt{System: "be terse", User: "hello", Temp: 0.2, MaxToks: 16})
	if err != nil {
		t.Fatalf("Generate() err = %v", err)
	}
	if out.Text != "hi" {
		t.Fatalf("Text = %q, want 'hi'", out.Text)
	}
	if out.Usage.Total != 4 {
		t.Fatalf("Total = %d, want 4", out.Usage.Total)
	}
	if captured.method != http.MethodPost {
		t.Fatalf("method = %s, want POST", captured.method)
	}
	if captured.path != "/chat/completions" {
		t.Fatalf("path = %s, want /chat/completions", captured.path)
	}
	if captured.auth != "Bearer sk-test" {
		t.Fatalf("auth = %s, want Bearer sk-test", captured.auth)
	}
	if captured.referer != "https://example.com" || captured.title != "smartjob" {
		t.Fatalf("openrouter headers missing: referer=%q title=%q", captured.referer, captured.title)
	}
	if captured.body.Model != "gpt-4o-mini" {
		t.Fatalf("body.Model = %s, want gpt-4o-mini", captured.body.Model)
	}
	if len(captured.body.Messages) != 2 ||
		captured.body.Messages[0].Role != "system" ||
		captured.body.Messages[1].Role != "user" {
		t.Fatalf("messages structure wrong: %+v", captured.body.Messages)
	}
	if captured.body.ResponseFormat != nil {
		t.Fatalf("ResponseFormat = %v, want nil for Generate", captured.body.ResponseFormat)
	}
}

func TestGenerateJSONSetsResponseFormat(t *testing.T) {
	var captured chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(chatResponse{
			Choices: []chatChoice{{Message: chatMessage{Role: "assistant", Content: `{"ok":true}`}}},
		})
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if _, err := c.GenerateJSON(context.Background(), ports.Prompt{User: "give json"}); err != nil {
		t.Fatalf("GenerateJSON() err = %v", err)
	}
	if captured.ResponseFormat == nil || captured.ResponseFormat.Type != "json_object" {
		t.Fatalf("ResponseFormat = %+v, want type=json_object", captured.ResponseFormat)
	}
}

func TestGenerateFailureCases(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantSub string
	}{
		{
			name:    "http_500_propagates_status",
			status:  500,
			body:    "boom",
			wantSub: "status 500",
		},
		{
			name:    "api_error_field_propagates",
			status:  200,
			body:    `{"error":{"message":"rate limited","type":"rate_limit"}}`,
			wantSub: "rate limited",
		},
		{
			name:    "empty_choices_is_error",
			status:  200,
			body:    `{"choices":[]}`,
			wantSub: "empty choices",
		},
		{
			name:    "non_json_body_is_decode_error",
			status:  200,
			body:    "not json",
			wantSub: "decode",
		},
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

func TestStreamNotImplemented(t *testing.T) {
	c, err := New(Config{APIKey: "k"})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if _, err := c.Stream(context.Background(), ports.Prompt{}); err == nil {
		t.Fatal("Stream() err = nil, want not-implemented error")
	}
}
