package reviewoutbox

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/my/app/internal/domain/ports"
)

func TestDelivererSignsAndPostsReviewItem(t *testing.T) {
	secret := "topsecret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/review-queue" {
			t.Fatalf("path = %s, want /internal/review-queue", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
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
			t.Fatalf("signature = %q, want %q", r.Header.Get("X-AI-Signature"), wantSig)
		}
		var got struct {
			CompanyID   int64           `json:"company_id"`
			Kind        string          `json:"kind"`
			Payload     json.RawMessage `json:"payload"`
			PayloadHash string          `json:"payload_hash"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got.CompanyID != 7 || got.Kind != "symptom_proposed" || string(got.Payload) != `{"name":"jam"}` || got.PayloadHash != "hash-1" {
			t.Fatalf("body = %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":123}}`))
	}))
	defer srv.Close()

	d, err := NewDeliverer(DeliveryConfig{BaseURL: srv.URL, Secret: secret, Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewDeliverer: %v", err)
	}
	ref, err := d.PushReviewItem(context.Background(), ports.ReviewOutboxItem{
		ID:          10,
		CompanyID:   7,
		Kind:        ports.ReviewKindSymptomPropose,
		Payload:     json.RawMessage(`{"name":"jam"}`),
		PayloadHash: "hash-1",
	})
	if err != nil {
		t.Fatalf("PushReviewItem: %v", err)
	}
	if ref != 123 {
		t.Fatalf("ref = %d, want 123", ref)
	}
}

func TestNewDelivererRejectsMissingConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  DeliveryConfig
		want string
	}{
		{name: "base", cfg: DeliveryConfig{Secret: "x"}, want: "base_url"},
		{name: "secret", cfg: DeliveryConfig{BaseURL: "http://x"}, want: "secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewDeliverer(tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
