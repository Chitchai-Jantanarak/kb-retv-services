package local

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/my/app/internal/domain/ports"
)

func TestNewRoutesToLocalBaseURLWithSentinelKey(t *testing.T) {
	var (
		auth string
		path string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		path = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL, Model: "llama3"})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	out, err := c.Generate(context.Background(), ports.Prompt{User: "hi"})
	if err != nil {
		t.Fatalf("Generate() err = %v", err)
	}
	if out.Text != "ok" {
		t.Fatalf("Text = %q, want ok", out.Text)
	}
	if auth != "Bearer "+sentinelKey {
		t.Fatalf("auth = %q, want Bearer %s", auth, sentinelKey)
	}
	if path != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", path)
	}
}

func TestNewDefaultsBaseURL(t *testing.T) {
	c, err := New(Config{Model: "m"})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if c == nil {
		t.Fatal("client = nil")
	}
}
