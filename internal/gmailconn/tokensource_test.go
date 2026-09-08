package gmailconn

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestAccessTokenFetchesAndCaches(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.PostForm.Get("refresh_token"); got != "rt-1" {
			t.Fatalf("refresh_token = %q, want rt-1", got)
		}
		if got := r.PostForm.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q, want refresh_token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"at-1","expires_in":3600}`))
	}))
	defer srv.Close()

	ts := NewTokenSource("cid", "secret", srv.URL, srv.Client())

	tok, err := ts.AccessToken(context.Background(), 1, "rt-1")
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if tok != "at-1" {
		t.Fatalf("token = %q, want at-1", tok)
	}

	// Second call within the TTL must hit the cache, not the server again.
	tok2, err := ts.AccessToken(context.Background(), 1, "rt-1")
	if err != nil {
		t.Fatalf("AccessToken (cached): %v", err)
	}
	if tok2 != "at-1" {
		t.Fatalf("cached token = %q, want at-1", tok2)
	}
	if calls != 1 {
		t.Fatalf("expected 1 token request (second call cached), got %d", calls)
	}
}

func TestAccessTokenInvalidGrantNeedsReconnect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	ts := NewTokenSource("cid", "secret", srv.URL, srv.Client())
	_, err := ts.AccessToken(context.Background(), 1, "rt-revoked")
	if !errors.Is(err, ErrNeedsReconnect) {
		t.Fatalf("err = %v, want ErrNeedsReconnect", err)
	}
}

func TestAccessTokenDifferentAccountsCachedSeparately(t *testing.T) {
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		calls[r.PostForm.Get("refresh_token")]++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"at-` + r.PostForm.Get("refresh_token") + `","expires_in":3600}`))
	}))
	defer srv.Close()

	ts := NewTokenSource("cid", "secret", srv.URL, srv.Client())
	tokA, err := ts.AccessToken(context.Background(), 1, "rt-a")
	if err != nil {
		t.Fatalf("AccessToken account 1: %v", err)
	}
	tokB, err := ts.AccessToken(context.Background(), 2, "rt-b")
	if err != nil {
		t.Fatalf("AccessToken account 2: %v", err)
	}
	if tokA == tokB {
		t.Fatalf("expected distinct tokens per account, got %q and %q", tokA, tokB)
	}
	if calls["rt-a"] != 1 || calls["rt-b"] != 1 {
		t.Fatalf("expected exactly one refresh per account, got %v", calls)
	}
}

func TestAccessTokenEmptyRefreshTokenRejected(t *testing.T) {
	ts := NewTokenSource("cid", "secret", "http://unused.invalid", http.DefaultClient)
	if _, err := ts.AccessToken(context.Background(), 1, "  "); err == nil {
		t.Fatal("expected error for empty refresh token")
	}
}

// sanity check that url.Values encoding round-trips the client id/secret too.
func TestAccessTokenSendsClientCredentials(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"at","expires_in":3600}`))
	}))
	defer srv.Close()

	ts := NewTokenSource("my-client-id", "my-client-secret", srv.URL, srv.Client())
	if _, err := ts.AccessToken(context.Background(), 1, "rt"); err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if got.Get("client_id") != "my-client-id" || got.Get("client_secret") != "my-client-secret" {
		t.Fatalf("client credentials not sent: %v", got)
	}
}
