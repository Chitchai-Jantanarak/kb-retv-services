package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
		_, _ = w.Write([]byte(`{"error":{"code":503,"status":"UNAVAILABLE","message":"high demand"}}`))
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
		path   string
		query  string
		apiKey string
		body   generateRequest
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.path = r.URL.Path
		captured.query = r.URL.RawQuery
		captured.apiKey = r.Header.Get("x-goog-api-key")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured.body)
		_ = json.NewEncoder(w).Encode(generateResponse{
			Candidates: []struct {
				Content content `json:"content"`
			}{
				{Content: content{Parts: []part{{Text: "ok"}}}},
			},
			UsageMetadata: generateUsage{PromptTokenCount: 3, CandidatesTokenCount: 1, TotalTokenCount: 4},
		})
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "gk", BaseURL: srv.URL, Model: "gemini-2.0-flash"})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	out, err := c.Generate(context.Background(), ports.Prompt{System: "be brief", User: "ping", Temp: 0.1, MaxToks: 8})
	if err != nil {
		t.Fatalf("Generate() err = %v", err)
	}
	if out.Text != "ok" {
		t.Fatalf("Text = %q, want ok", out.Text)
	}
	if out.Usage.Total != 4 {
		t.Fatalf("Total = %d, want 4", out.Usage.Total)
	}
	if !strings.Contains(captured.path, "models/gemini-2.0-flash:generateContent") {
		t.Fatalf("path = %s", captured.path)
	}
	if captured.apiKey != "gk" {
		t.Fatalf("x-goog-api-key = %s, want gk", captured.apiKey)
	}
	if strings.Contains(captured.query, "key=") {
		t.Fatalf("query = %s, api key must not ride the URL", captured.query)
	}
	if captured.body.SystemInstruction == nil || captured.body.SystemInstruction.Parts[0].Text != "be brief" {
		t.Fatalf("systemInstruction wrong: %+v", captured.body.SystemInstruction)
	}
	if captured.body.GenerationConfig == nil || captured.body.GenerationConfig.MaxOutputTokens != 8 {
		t.Fatalf("generationConfig wrong: %+v", captured.body.GenerationConfig)
	}
}

func TestGenerateUsesCurrentDefaultModel(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(generateResponse{
			Candidates: []struct {
				Content content `json:"content"`
			}{
				{Content: content{Parts: []part{{Text: "ok"}}}},
			},
		})
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "gk", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if _, err := c.Generate(context.Background(), ports.Prompt{User: "ping"}); err != nil {
		t.Fatalf("Generate() err = %v", err)
	}
	if !strings.Contains(capturedPath, "models/gemini-2.5-flash:generateContent") {
		t.Fatalf("path = %s, want gemini-2.5-flash", capturedPath)
	}
}

func TestGenerateJSONSetsResponseMimeType(t *testing.T) {
	var captured generateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(generateResponse{
			Candidates: []struct {
				Content content `json:"content"`
			}{
				{Content: content{Parts: []part{{Text: `{"ok":true}`}}}},
			},
		})
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if _, err := c.GenerateJSON(context.Background(), ports.Prompt{User: "json please"}); err != nil {
		t.Fatalf("GenerateJSON() err = %v", err)
	}
	if captured.GenerationConfig == nil || captured.GenerationConfig.ResponseMimeType != "application/json" {
		t.Fatalf("responseMimeType = %+v, want application/json", captured.GenerationConfig)
	}
}

func TestStreamForwardsSSEDeltasAndCloses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hel\"}]}}]}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"lo\"}]}}]}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	ch, err := c.Stream(context.Background(), ports.Prompt{User: "hi"})
	if err != nil {
		t.Fatalf("Stream() err = %v", err)
	}

	var got []string
	for chunk := range ch {
		got = append(got, chunk.Text)
	}
	if strings.Join(got, "") != "Hello" {
		t.Fatalf("fragments = %v, want joined 'Hello'", got)
	}
}

func TestStreamNonOKStatusReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":503,"message":"overloaded"}}`))
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if _, err := c.Stream(context.Background(), ports.Prompt{User: "x"}); err == nil {
		t.Fatal("Stream() err = nil, want error on non-200")
	}
}

func TestGenerateSendsInlineImagePart(t *testing.T) {
	pngBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	var captured generateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(generateResponse{
			Candidates: []struct {
				Content content `json:"content"`
			}{
				{Content: content{Parts: []part{{Text: "ok"}}}},
			},
		})
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	_, err = c.Generate(context.Background(), ports.Prompt{
		User: "describe this",
		Images: []ports.PromptImage{
			{MIMEType: "image/png", Data: pngBytes},
		},
	})
	if err != nil {
		t.Fatalf("Generate() err = %v", err)
	}

	if len(captured.Contents) != 1 {
		t.Fatalf("Contents len = %d, want 1", len(captured.Contents))
	}
	parts := captured.Contents[0].Parts
	if len(parts) != 2 {
		t.Fatalf("Parts len = %d, want 2", len(parts))
	}
	if parts[0].Text != "describe this" {
		t.Fatalf("Parts[0].Text = %q, want 'describe this' (text must stay first)", parts[0].Text)
	}
	if parts[1].InlineData == nil {
		t.Fatal("Parts[1].InlineData = nil, want populated inlineData")
	}
	if parts[1].InlineData.MIMEType != "image/png" {
		t.Fatalf("InlineData.MIMEType = %q, want image/png", parts[1].InlineData.MIMEType)
	}
	wantData := base64.StdEncoding.EncodeToString(pngBytes)
	if parts[1].InlineData.Data != wantData {
		t.Fatalf("InlineData.Data = %q, want %q", parts[1].InlineData.Data, wantData)
	}
}

func TestGenerateNoImagesOmitsInlineData(t *testing.T) {
	var rawBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawBody, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(generateResponse{
			Candidates: []struct {
				Content content `json:"content"`
			}{
				{Content: content{Parts: []part{{Text: "ok"}}}},
			},
		})
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if _, err := c.Generate(context.Background(), ports.Prompt{User: "ping"}); err != nil {
		t.Fatalf("Generate() err = %v", err)
	}

	if strings.Contains(string(rawBody), "inlineData") {
		t.Fatalf("body = %s, want no inlineData key when Images is empty", rawBody)
	}
	want, _ := json.Marshal(generateRequest{
		Contents: []content{{Role: "user", Parts: []part{{Text: "ping"}}}},
	})
	if string(rawBody) != string(want) {
		t.Fatalf("body = %s, want %s", rawBody, want)
	}
}

func TestGenerateSkipsOversizedImage(t *testing.T) {
	orig := maxPromptImageBytes
	maxPromptImageBytes = 4
	defer func() { maxPromptImageBytes = orig }()

	var captured generateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(generateResponse{
			Candidates: []struct {
				Content content `json:"content"`
			}{
				{Content: content{Parts: []part{{Text: "ok"}}}},
			},
		})
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	out, err := c.Generate(context.Background(), ports.Prompt{
		User: "ping",
		Images: []ports.PromptImage{
			{MIMEType: "image/png", Data: []byte{1, 2, 3, 4, 5}},
		},
	})
	if err != nil {
		t.Fatalf("Generate() err = %v, want success (oversized image skipped, not fatal)", err)
	}
	if out.Text != "ok" {
		t.Fatalf("Text = %q, want ok", out.Text)
	}
	if len(captured.Contents) != 1 || len(captured.Contents[0].Parts) != 1 {
		t.Fatalf("Parts = %+v, want text-only single part", captured.Contents[0].Parts)
	}
}

func TestGenerateCapsImagesAtMax(t *testing.T) {
	orig := maxPromptImages
	maxPromptImages = 2
	defer func() { maxPromptImages = orig }()

	var captured generateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(generateResponse{
			Candidates: []struct {
				Content content `json:"content"`
			}{
				{Content: content{Parts: []part{{Text: "ok"}}}},
			},
		})
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	_, err = c.Generate(context.Background(), ports.Prompt{
		User: "ping",
		Images: []ports.PromptImage{
			{MIMEType: "image/png", Data: []byte("one")},
			{MIMEType: "image/png", Data: []byte("two")},
			{MIMEType: "image/png", Data: []byte("three")},
		},
	})
	if err != nil {
		t.Fatalf("Generate() err = %v", err)
	}
	parts := captured.Contents[0].Parts
	if len(parts) != 3 {
		t.Fatalf("Parts len = %d, want 3 (1 text + 2 images)", len(parts))
	}
	if parts[1].InlineData.Data != base64.StdEncoding.EncodeToString([]byte("one")) {
		t.Fatalf("first image = %q, want 'one'", parts[1].InlineData.Data)
	}
	if parts[2].InlineData.Data != base64.StdEncoding.EncodeToString([]byte("two")) {
		t.Fatalf("second image = %q, want 'two'", parts[2].InlineData.Data)
	}
}

func TestGenerateSkipsNonImageMIMEType(t *testing.T) {
	var captured generateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(generateResponse{
			Candidates: []struct {
				Content content `json:"content"`
			}{
				{Content: content{Parts: []part{{Text: "ok"}}}},
			},
		})
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	_, err = c.Generate(context.Background(), ports.Prompt{
		User: "ping",
		Images: []ports.PromptImage{
			{MIMEType: "application/pdf", Data: []byte("not an image")},
		},
	})
	if err != nil {
		t.Fatalf("Generate() err = %v", err)
	}
	if len(captured.Contents[0].Parts) != 1 {
		t.Fatalf("Parts = %+v, want text-only single part", captured.Contents[0].Parts)
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
		{name: "api_error", status: 200, body: `{"error":{"code":429,"message":"quota exceeded","status":"RESOURCE_EXHAUSTED"}}`, wantSub: "quota exceeded"},
		{name: "empty_candidates", status: 200, body: `{"candidates":[]}`, wantSub: "empty candidates"},
		{name: "no_text_parts", status: 200, body: `{"candidates":[{"content":{"parts":[]}}]}`, wantSub: "no text parts"},
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
