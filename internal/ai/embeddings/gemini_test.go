package embeddings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeminiProviderEmbedsTexts(t *testing.T) {
	var sawKey bool
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1beta/models/gemini-embedding-2:batchEmbedContents" {
			t.Fatalf("path = %s, want /v1beta/models/gemini-embedding-2:batchEmbedContents", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") == "gemini-key" {
			sawKey = true
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"embeddings": [
				{"values": [0.1, 0.2, 0.3]},
				{"values": [0.4, 0.5, 0.6]}
			]
		}`))
	}))
	defer server.Close()

	provider := NewGeminiProvider(GeminiConfig{
		BaseURL:    server.URL + "/v1beta",
		APIKey:     "gemini-key",
		Model:      "gemini-embedding-2",
		Dimensions: 3,
	})
	vectors, err := provider.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if !sawKey {
		t.Fatal("x-goog-api-key header was not sent")
	}
	requests := requestBody["requests"].([]any)
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	first := requests[0].(map[string]any)
	if first["model"] != "models/gemini-embedding-2" {
		t.Fatalf("model = %v, want models/gemini-embedding-2", first["model"])
	}
	if first["outputDimensionality"] != float64(3) {
		t.Fatalf("outputDimensionality = %v, want 3", first["outputDimensionality"])
	}
	if len(vectors) != 2 {
		t.Fatalf("vectors = %d, want 2", len(vectors))
	}
	if vectors[1][0] != 0.4 {
		t.Fatalf("vectors[1][0] = %v, want 0.4", vectors[1][0])
	}
	if provider.Dim() != 3 {
		t.Fatalf("Dim() = %d, want 3", provider.Dim())
	}
}

func TestGeminiProviderDefaultsDimension(t *testing.T) {
	provider := NewGeminiProvider(GeminiConfig{})

	if provider.Dim() != 3072 {
		t.Fatalf("Dim() = %d, want 3072", provider.Dim())
	}
}
