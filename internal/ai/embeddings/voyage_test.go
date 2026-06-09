package embeddings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVoyageProviderEmbedsTexts(t *testing.T) {
	var sawAuth bool
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("path = %s, want /v1/embeddings", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "Bearer voyage-key" {
			sawAuth = true
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"data": [
				{"index": 0, "embedding": [0.1, 0.2]},
				{"index": 1, "embedding": [0.3, 0.4]}
			],
			"model": "voyage-4"
		}`))
	}))
	defer server.Close()

	provider := NewVoyageProvider(VoyageConfig{
		BaseURL:    server.URL + "/v1",
		APIKey:     "voyage-key",
		Model:      "voyage-4",
		Dimensions: 2,
	})
	vectors, err := provider.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if !sawAuth {
		t.Fatal("Authorization header was not sent")
	}
	if requestBody["model"] != "voyage-4" {
		t.Fatalf("model = %v, want voyage-4", requestBody["model"])
	}
	if requestBody["input_type"] != "document" {
		t.Fatalf("input_type = %v, want document", requestBody["input_type"])
	}
	if requestBody["output_dimension"] != float64(2) {
		t.Fatalf("output_dimension = %v, want 2", requestBody["output_dimension"])
	}
	if len(vectors) != 2 {
		t.Fatalf("vectors = %d, want 2", len(vectors))
	}
	if vectors[1][0] != 0.3 {
		t.Fatalf("vectors[1][0] = %v, want 0.3", vectors[1][0])
	}
}
