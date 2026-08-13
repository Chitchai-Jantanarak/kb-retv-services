package mediastore

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/my/app/internal/application/dto"
)

func TestDeliverValidatesSignature(t *testing.T) {
	secret := "shhh"
	var gotBody []byte
	var gotTimestamp, gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		gotTimestamp = r.Header.Get("X-AI-Timestamp")
		gotSig = r.Header.Get("X-AI-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d, err := NewDeliverer(DeliveryConfig{BaseURL: srv.URL, Secret: secret})
	if err != nil {
		t.Fatalf("NewDeliverer: %v", err)
	}
	payload := Payload{CompanyID: 7, ConversationID: 100, MessageID: 200, ExternalMessageID: "m-1", MIMEType: "image/jpeg", DataBase64: "Zm9v"}
	if err := d.Deliver(context.Background(), payload); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(gotTimestamp))
	mac.Write([]byte("."))
	mac.Write(gotBody)
	wantSig := hex.EncodeToString(mac.Sum(nil))
	if gotSig != wantSig {
		t.Fatalf("signature = %q, want %q", gotSig, wantSig)
	}

	var decoded Payload
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if decoded.CompanyID != 7 || decoded.MessageID != 200 {
		t.Fatalf("decoded payload = %+v", decoded)
	}
}

func TestDeliverErrorsOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d, err := NewDeliverer(DeliveryConfig{BaseURL: srv.URL, Secret: "shhh"})
	if err != nil {
		t.Fatalf("NewDeliverer: %v", err)
	}
	err = d.Deliver(context.Background(), Payload{CompanyID: 1})
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("err = %v, want status 500", err)
	}
}

type stubFetcher struct {
	data  []byte
	mime  string
	err   error
	gotID string
}

func (s *stubFetcher) FetchContent(_ context.Context, messageID string) ([]byte, string, error) {
	s.gotID = messageID
	return s.data, s.mime, s.err
}

func TestPromoterFetchesAndDelivers(t *testing.T) {
	var gotPayload Payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d, err := NewDeliverer(DeliveryConfig{BaseURL: srv.URL, Secret: "shhh"})
	if err != nil {
		t.Fatalf("NewDeliverer: %v", err)
	}
	fetcher := &stubFetcher{data: []byte("img-bytes"), mime: "image/jpeg"}
	p := NewPromoter(fetcher, d)

	ref := dto.AttachmentRef{ID: "line-msg-1", MIMEType: "image/jpeg"}
	if err := p.Promote(context.Background(), 7, 100, 200, ref); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if fetcher.gotID != "line-msg-1" {
		t.Fatalf("fetcher got id = %q, want line-msg-1", fetcher.gotID)
	}
	if gotPayload.CompanyID != 7 || gotPayload.ConversationID != 100 || gotPayload.MessageID != 200 {
		t.Fatalf("payload ids = %+v", gotPayload)
	}
	if gotPayload.ExternalMessageID != "line-msg-1" {
		t.Fatalf("external_message_id = %q, want line-msg-1", gotPayload.ExternalMessageID)
	}
	if gotPayload.MIMEType != "image/jpeg" {
		t.Fatalf("mime = %q, want image/jpeg", gotPayload.MIMEType)
	}
	wantB64 := base64.StdEncoding.EncodeToString([]byte("img-bytes"))
	if gotPayload.DataBase64 != wantB64 {
		t.Fatalf("data_base64 = %q, want %q", gotPayload.DataBase64, wantB64)
	}
}

func TestPromoterFallsBackToRefMIMEType(t *testing.T) {
	var gotPayload Payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d, err := NewDeliverer(DeliveryConfig{BaseURL: srv.URL, Secret: "shhh"})
	if err != nil {
		t.Fatalf("NewDeliverer: %v", err)
	}
	fetcher := &stubFetcher{data: []byte("audio-bytes"), mime: ""}
	p := NewPromoter(fetcher, d)

	ref := dto.AttachmentRef{ID: "line-msg-2", MIMEType: "audio/m4a"}
	if err := p.Promote(context.Background(), 7, 100, 201, ref); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if gotPayload.MIMEType != "audio/m4a" {
		t.Fatalf("mime = %q, want fallback audio/m4a", gotPayload.MIMEType)
	}
}

func TestPromoterFetchFailurePropagates(t *testing.T) {
	fetcher := &stubFetcher{err: errStub("line api down")}
	p := NewPromoter(fetcher, &Deliverer{})

	err := p.Promote(context.Background(), 7, 100, 200, dto.AttachmentRef{ID: "line-msg-3"})
	if err == nil || !strings.Contains(err.Error(), "line api down") {
		t.Fatalf("err = %v, want to wrap fetch failure", err)
	}
}

type errStub string

func (e errStub) Error() string { return string(e) }

func TestPromoteBytesDelivers(t *testing.T) {
	var gotPayload Payload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d, err := NewDeliverer(DeliveryConfig{BaseURL: srv.URL, Secret: "shhh"})
	if err != nil {
		t.Fatalf("NewDeliverer: %v", err)
	}
	p := NewPromoter(&stubFetcher{}, d)

	err = p.PromoteBytes(context.Background(), 7, 100, 200, "msg-1#0", "image/png", []byte("email-bytes"))
	if err != nil {
		t.Fatalf("PromoteBytes: %v", err)
	}
	if gotPayload.CompanyID != 7 || gotPayload.ConversationID != 100 || gotPayload.MessageID != 200 {
		t.Fatalf("payload ids = %+v", gotPayload)
	}
	if gotPayload.ExternalMessageID != "msg-1#0" {
		t.Fatalf("external_message_id = %q, want msg-1#0", gotPayload.ExternalMessageID)
	}
	if gotPayload.MIMEType != "image/png" {
		t.Fatalf("mime = %q, want image/png", gotPayload.MIMEType)
	}
	wantB64 := base64.StdEncoding.EncodeToString([]byte("email-bytes"))
	if gotPayload.DataBase64 != wantB64 {
		t.Fatalf("data_base64 = %q, want %q", gotPayload.DataBase64, wantB64)
	}
}

func TestPromoteBytesPropagatesDeliveryFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d, err := NewDeliverer(DeliveryConfig{BaseURL: srv.URL, Secret: "shhh"})
	if err != nil {
		t.Fatalf("NewDeliverer: %v", err)
	}
	p := NewPromoter(&stubFetcher{}, d)

	err = p.PromoteBytes(context.Background(), 7, 100, 200, "msg-1#0", "image/png", []byte("email-bytes"))
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("err = %v, want status 500", err)
	}
}
