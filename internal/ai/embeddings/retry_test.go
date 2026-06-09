package embeddings

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetrierRetriesOn429ThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":{"message":"rate limited"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"embeddings":[{"values":[0.1,0.2,0.3]}]}`))
	}))
	defer server.Close()

	provider := NewGeminiProvider(GeminiConfig{
		BaseURL:    server.URL + "/v1beta",
		APIKey:     "key",
		Model:      "gemini-embedding-2",
		Dimensions: 3,
		MaxRetries: 3,
	})

	vectors, err := provider.Embed(context.Background(), []string{"a"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("server calls = %d, want 2", got)
	}
	if len(vectors) != 1 || vectors[0][0] != 0.1 {
		t.Fatalf("vectors = %v, want [[0.1 0.2 0.3]]", vectors)
	}
}

func TestRetrierGivesUpAfterMaxRetries(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"quota exceeded"}}`))
	}))
	defer server.Close()

	provider := NewGeminiProvider(GeminiConfig{
		BaseURL:    server.URL + "/v1beta",
		APIKey:     "key",
		Model:      "gemini-embedding-2",
		Dimensions: 3,
		MaxRetries: 1,
	})

	_, err := provider.Embed(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("Embed() error = nil, want error after retries exhausted")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("server calls = %d, want 2 (1 try + 1 retry)", got)
	}
}

func TestBackoffHonorsRetryAfter(t *testing.T) {
	wait := backoffDuration(0, 5*time.Second)
	if wait < 5*time.Second {
		t.Fatalf("backoff = %v, want >= 5s when Retry-After is 5s", wait)
	}
}
