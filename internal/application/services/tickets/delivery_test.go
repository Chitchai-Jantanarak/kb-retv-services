package tickets

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewDelivererRejectsMissingConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  DeliveryConfig
		want string
	}{
		{"no_base", DeliveryConfig{Secret: "x"}, "base_url"},
		{"no_secret", DeliveryConfig{BaseURL: "http://x"}, "secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewDeliverer(tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want sub %q", err, tc.want)
			}
		})
	}
}

func TestDelivererSignsAndPosts(t *testing.T) {
	body := []byte(`{"company_id":7}`)
	secret := "topsecret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/webhooks/ai/ticket-create" {
			t.Errorf("path = %s", r.URL.Path)
		}
		got, _ := io.ReadAll(r.Body)
		if string(got) != string(body) {
			t.Errorf("body = %s", string(got))
		}
		timestamp := r.Header.Get("X-AI-Timestamp")
		if timestamp == "" {
			t.Fatal("missing X-AI-Timestamp")
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(timestamp))
		mac.Write([]byte("."))
		mac.Write(body)
		wantSig := hex.EncodeToString(mac.Sum(nil))
		if r.Header.Get("X-AI-Signature") != wantSig {
			t.Errorf("sig = %s want %s", r.Header.Get("X-AI-Signature"), wantSig)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	d, err := NewDeliverer(DeliveryConfig{BaseURL: srv.URL, Secret: secret, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewDeliverer: %v", err)
	}
	if err := d.Deliver(context.Background(), body); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
}

func TestDelivererReportsErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()
	d, err := NewDeliverer(DeliveryConfig{BaseURL: srv.URL, Secret: "x"})
	if err != nil {
		t.Fatalf("NewDeliverer: %v", err)
	}
	err = d.Deliver(context.Background(), []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v", err)
	}
}

func TestDelivererRequiresBody(t *testing.T) {
	d, _ := NewDeliverer(DeliveryConfig{BaseURL: "http://x", Secret: "y"})
	err := d.Deliver(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "empty body") {
		t.Fatalf("expected empty-body error, got %v", err)
	}
}
