package mediastore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/my/app/internal/application/dto"
)

func TestPromoteWithoutFetcherReturnsSentinel(t *testing.T) {
	p := NewPromoter(nil, &Deliverer{})
	err := p.Promote(context.Background(), 1, 2, 3, dto.AttachmentRef{ID: "att-1", MIMEType: "image/png"})
	if !errors.Is(err, ErrNoContentFetcher) {
		t.Fatalf("Promote() error = %v, want ErrNoContentFetcher", err)
	}
}

func TestPromoteBytesWorksWithoutFetcher(t *testing.T) {
	delivered := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delivered = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d, err := NewDeliverer(DeliveryConfig{BaseURL: srv.URL, Secret: "shhh"})
	if err != nil {
		t.Fatalf("NewDeliverer: %v", err)
	}
	p := NewPromoter(nil, d)

	if err := p.PromoteBytes(context.Background(), 1, 2, 3, "msg-1#0", "image/png", []byte("bytes")); err != nil {
		t.Fatalf("PromoteBytes() error = %v, want nil", err)
	}
	if !delivered {
		t.Fatal("PromoteBytes did not reach the deliverer")
	}
}
