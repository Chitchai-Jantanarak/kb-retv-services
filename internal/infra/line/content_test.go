package line

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchContentHappyPath(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("binary-image-bytes"))
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token", 0)
	data, mime, err := c.FetchContent(context.Background(), "msg-123")
	if err != nil {
		t.Fatalf("FetchContent: %v", err)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("Authorization header = %q, want Bearer test-token", gotAuth)
	}
	if gotPath != "/v2/bot/message/msg-123/content" {
		t.Fatalf("path = %q, want /v2/bot/message/msg-123/content", gotPath)
	}
	if string(data) != "binary-image-bytes" {
		t.Fatalf("data = %q", data)
	}
	if mime != "image/jpeg" {
		t.Fatalf("mime = %q, want image/jpeg", mime)
	}
}

func TestFetchContentNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token", 0)
	_, _, err := c.FetchContent(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("err = %v, want status 404", err)
	}
}

func TestFetchContentOversizeRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		big := make([]byte, maxContentBytes+1024)
		_, _ = w.Write(big)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-token", 0)
	_, _, err := c.FetchContent(context.Background(), "big")
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("err = %v, want too large", err)
	}
}
