package mailpoll

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestForwardSignsAndPosts(t *testing.T) {
	var gotSig string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-AI-Signature")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	secret := "test-secret"
	f := NewForwarder(srv.URL, secret, &http.Client{Timeout: 5 * time.Second})
	p := EmailPayload{MessageID: "<m-1>", From: "a@b.com", To: "in@x.com", Subject: "hi", Body: "yo"}

	if err := f.Forward(context.Background(), p); err != nil {
		t.Fatalf("forward: %v", err)
	}

	want := "sha256=" + Sign(secret, gotBody)
	if gotSig != want {
		t.Fatalf("sig = %q, want %q", gotSig, want)
	}
}

func TestForwardReturnsErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	f := NewForwarder(srv.URL, "s", srv.Client())
	if err := f.Forward(context.Background(), EmailPayload{}); err == nil {
		t.Fatal("expected error on 401")
	}
}
