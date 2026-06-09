package embeddings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenRouterProviderEmbedsTexts(t *testing.T) {
	var requestBody map[string]any
	var sawAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/embeddings" {
			t.Fatalf("path = %s, want /api/v1/embeddings", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "Bearer openrouter-key" {
			sawAuth = true
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"data": [
				{"index": 0, "embedding": [0.1, 0.2, 0.3]},
				{"index": 1, "embedding": [0.4, 0.5, 0.6]}
			]
		}`))
	}))
	defer server.Close()

	provider := NewOpenRouterProvider(OpenRouterConfig{
		BaseURL: server.URL + "/api/v1",
		APIKey:  "openrouter-key",
		Model:   "openai/text-embedding-3-small",
	})
	vectors, err := provider.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if !sawAuth {
		t.Fatal("Authorization header was not sent")
	}
	if requestBody["model"] != "openai/text-embedding-3-small" {
		t.Fatalf("model = %v, want openai/text-embedding-3-small", requestBody["model"])
	}
	if _, ok := requestBody["encoding_format"]; ok {
		t.Fatal("encoding_format was sent")
	}
	if len(vectors) != 2 {
		t.Fatalf("vectors = %d, want 2", len(vectors))
	}
	if provider.Dim() != 1536 {
		t.Fatalf("Dim() = %d, want 1536", provider.Dim())
	}
}
