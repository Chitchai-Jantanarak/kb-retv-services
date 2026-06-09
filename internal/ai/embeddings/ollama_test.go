package embeddings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaProviderEmbedsTexts(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/embed" {
			t.Fatalf("path = %s, want /api/embed", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"model": "all-minilm",
			"embeddings": [
				[0.1, 0.2],
				[0.3, 0.4]
			]
		}`))
	}))
	defer server.Close()

	provider := NewOllamaProvider(OllamaConfig{
		BaseURL: server.URL,
		Model:   "all-minilm",
	})
	vectors, err := provider.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if requestBody["model"] != "all-minilm" {
		t.Fatalf("model = %v, want all-minilm", requestBody["model"])
	}
	if len(requestBody["input"].([]any)) != 2 {
		t.Fatalf("input length = %d, want 2", len(requestBody["input"].([]any)))
	}
	if len(vectors) != 2 {
		t.Fatalf("vectors = %d, want 2", len(vectors))
	}
	if provider.Dim() != 384 {
		t.Fatalf("Dim() = %d, want 384", provider.Dim())
	}
}
