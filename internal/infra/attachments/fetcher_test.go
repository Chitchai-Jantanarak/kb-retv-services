package attachments

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/my/app/internal/domain/ports"
	apperr "github.com/my/app/internal/shared/errors"
)

func TestFetchHappyPathRelativeURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("fake-bytes"))
	}))
	defer server.Close()

	c := New(server.URL, "", 0, 0)
	data, err := c.Fetch(context.Background(), ports.Attachment{URL: "/api/ai/attachments/fetch?key=x"})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if string(data.Bytes) != "fake-bytes" {
		t.Fatalf("Bytes = %q, want fake-bytes", data.Bytes)
	}
	if data.MIMEType != "image/png" {
		t.Fatalf("MIMEType = %q, want image/png", data.MIMEType)
	}
}

func TestFetchNon200ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := New(server.URL, "", 0, 0)
	_, err := c.Fetch(context.Background(), ports.Attachment{URL: "/missing"})
	if err == nil {
		t.Fatal("Fetch() error = nil, want error")
	}
}

func TestFetchTooLargeReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 20))
	}))
	defer server.Close()

	c := New(server.URL, "", 0, 10)
	_, err := c.Fetch(context.Background(), ports.Attachment{URL: "/big"})
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodePayloadTooLarge {
		t.Fatalf("Fetch() error = %v, want payload_too_large", err)
	}
}

func TestFetchSetsHostHeader(t *testing.T) {
	var gotHost string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	c := New(server.URL, "app.internal.test", 0, 0)
	_, err := c.Fetch(context.Background(), ports.Attachment{URL: "/x"})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if gotHost != "app.internal.test" {
		t.Fatalf("Host = %q, want app.internal.test", gotHost)
	}
}
