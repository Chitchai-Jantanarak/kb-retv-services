package embeddings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIProviderEmbedsTexts(t *testing.T) {
	var sawAuth bool
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("path = %s, want /v1/embeddings", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "Bearer test-key" {
			sawAuth = true
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"data": [
				{"index": 1, "embedding": [0.3, 0.4]},
				{"index": 0, "embedding": [0.1, 0.2]}
			],
			"model": "text-embedding-3-small"
		}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(OpenAIConfig{
		BaseURL:    server.URL + "/v1",
		APIKey:     "test-key",
		Model:      "text-embedding-3-small",
		Dimensions: 2,
	})
	vectors, err := provider.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if !sawAuth {
		t.Fatal("Authorization header was not sent")
	}
	if requestBody["model"] != "text-embedding-3-small" {
		t.Fatalf("model = %v, want text-embedding-3-small", requestBody["model"])
	}
	if requestBody["encoding_format"] != "float" {
		t.Fatalf("encoding_format = %v, want float", requestBody["encoding_format"])
	}
	if requestBody["dimensions"] != float64(2) {
		t.Fatalf("dimensions = %v, want 2", requestBody["dimensions"])
	}
	if len(vectors) != 2 {
		t.Fatalf("vectors = %d, want 2", len(vectors))
	}
	if vectors[0][0] != 0.1 || vectors[1][0] != 0.3 {
		t.Fatalf("vectors not ordered by index: %#v", vectors)
	}
	if provider.Dim() != 2 {
		t.Fatalf("Dim() = %d, want 2", provider.Dim())
	}
}

func TestOpenAIProviderDefaultsSmallModelDimension(t *testing.T) {
	provider := NewOpenAIProvider(OpenAIConfig{Model: "text-embedding-3-small"})

	if provider.Dim() != 1536 {
		t.Fatalf("Dim() = %d, want 1536", provider.Dim())
	}
}
