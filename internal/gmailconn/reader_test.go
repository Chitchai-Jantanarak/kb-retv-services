package gmailconn

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/my/app/internal/mailpoll"
	"github.com/my/app/internal/shared/providercrypto"
)

func testProviderKey() []byte { return bytes.Repeat([]byte{0x7A}, 32) }

type fakeStore struct {
	mu           sync.Mutex
	accounts     []OAuthAccount
	savedHistory map[int64]string
	states       map[int64]string
}

func (f *fakeStore) ListOAuthGmail(context.Context) ([]OAuthAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]OAuthAccount, len(f.accounts))
	copy(out, f.accounts)
	for i := range out {
		if h, ok := f.savedHistory[out[i].ID]; ok {
			out[i].HistoryID = h
		}
	}
	return out, nil
}

func (f *fakeStore) SaveHistoryID(_ context.Context, id int64, historyID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.savedHistory == nil {
		f.savedHistory = map[int64]string{}
	}
	f.savedHistory[id] = historyID
	return nil
}

func (f *fakeStore) MarkOAuthState(_ context.Context, id int64, state string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.states == nil {
		f.states = map[int64]string{}
	}
	f.states[id] = state
	return nil
}

func (f *fakeStore) stateOf(id int64) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.states[id]
}

func (f *fakeStore) historyOf(id int64) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.savedHistory[id]
}

// forwardRecorder is a fake inbound webhook: it records every payload posted
// to it, standing in for the Laravel /v1/inbound/email endpoint.
type forwardRecorder struct {
	mu       sync.Mutex
	payloads []mailpoll.EmailPayload
}

func (r *forwardRecorder) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var p mailpoll.EmailPayload
		if err := json.NewDecoder(req.Body).Decode(&p); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		r.mu.Lock()
		r.payloads = append(r.payloads, p)
		r.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
}

func (r *forwardRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.payloads)
}

func rawMessage(from, to, subject, body string) []byte {
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		from, to, subject, body)
	return []byte(msg)
}

// newGmailServer fakes the three Gmail REST endpoints the reader uses. Every
// call to /users/me/history returns the same fixed set of message ids and
// historyId (simulating Gmail resending an overlapping history window), so
// tests can exercise the reader's own in-memory dedupe.
func newGmailServer(t *testing.T, historyID string, ids []string, rawByID map[string][]byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/users/me/history", func(w http.ResponseWriter, r *http.Request) {
		var added []string
		for _, id := range ids {
			added = append(added, fmt.Sprintf(`{"message":{"id":%q}}`, id))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"history":[{"messagesAdded":[%s]}],"historyId":%q}`, strings.Join(added, ","), historyID)
	})
	mux.HandleFunc("/users/me/messages/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/users/me/messages/")
		raw, ok := rawByID[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%q,"raw":%q}`, id, base64.RawURLEncoding.EncodeToString(raw))
	})
	return httptest.NewServer(mux)
}

func newTokenServer(t *testing.T, accessToken string, invalidGrant bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if invalidGrant {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":%q,"expires_in":3600}`, accessToken)
	}))
}

func newTestReader(t *testing.T, store AccountStore, tokenSrv, gmailSrv *httptest.Server, fwdURL string) *Reader {
	t.Helper()
	tokens := NewTokenSource("cid", "secret", tokenSrv.URL, tokenSrv.Client())
	client := NewClient(gmailSrv.URL, gmailSrv.Client())
	fwd := mailpoll.NewForwarder(fwdURL, "test-secret", &http.Client{Timeout: 5 * time.Second})
	return NewReader(store, tokens, client, fwd, testProviderKey(), time.Minute, "")
}

func encryptedRefreshToken(t *testing.T, plain string) string {
	t.Helper()
	enc, err := providercrypto.Encrypt(plain, testProviderKey())
	if err != nil {
		t.Fatalf("encrypt refresh token: %v", err)
	}
	return enc
}

func TestPollAccountForwardsNewMessagesAndSavesHistoryID(t *testing.T) {
	own := "tenant@gmail.com"
	rawByID := map[string][]byte{
		"m1": rawMessage("customer1@example.com", own, "issue one", "body one"),
		"m2": rawMessage("customer2@example.com", own, "issue two", "body two"),
	}
	gmailSrv := newGmailServer(t, "200", []string{"m1", "m2"}, rawByID)
	defer gmailSrv.Close()
	tokenSrv := newTokenServer(t, "at-1", false)
	defer tokenSrv.Close()
	rec := &forwardRecorder{}
	fwdSrv := rec.server()
	defer fwdSrv.Close()

	store := &fakeStore{accounts: []OAuthAccount{
		{ID: 1, CompanyID: 7, ExternalID: own, RefreshEnc: encryptedRefreshToken(t, "rt-1"), HistoryID: "100"},
	}}
	r := newTestReader(t, store, tokenSrv, gmailSrv, fwdSrv.URL)

	r.pollOnce(context.Background())

	if got := rec.count(); got != 2 {
		t.Fatalf("forwarded %d messages, want 2", got)
	}
	if got := store.historyOf(1); got != "200" {
		t.Fatalf("saved historyId = %q, want 200", got)
	}
}

func TestPollAccountSkipsSelfSent(t *testing.T) {
	own := "tenant@gmail.com"
	rawByID := map[string][]byte{
		"m1": rawMessage(own, "someone@example.com", "auto reply", "body"),
	}
	gmailSrv := newGmailServer(t, "200", []string{"m1"}, rawByID)
	defer gmailSrv.Close()
	tokenSrv := newTokenServer(t, "at-1", false)
	defer tokenSrv.Close()
	rec := &forwardRecorder{}
	fwdSrv := rec.server()
	defer fwdSrv.Close()

	store := &fakeStore{accounts: []OAuthAccount{
		{ID: 1, CompanyID: 7, ExternalID: own, RefreshEnc: encryptedRefreshToken(t, "rt-1"), HistoryID: "100"},
	}}
	r := newTestReader(t, store, tokenSrv, gmailSrv, fwdSrv.URL)

	r.pollOnce(context.Background())

	if got := rec.count(); got != 0 {
		t.Fatalf("forwarded %d messages, want 0 (self-sent must be skipped)", got)
	}
	// historyId still advances even though the only message was skipped.
	if got := store.historyOf(1); got != "200" {
		t.Fatalf("saved historyId = %q, want 200", got)
	}
}

func TestPollAccountDedupeOnRepeatHistory(t *testing.T) {
	own := "tenant@gmail.com"
	rawByID := map[string][]byte{
		"m1": rawMessage("customer@example.com", own, "hello", "body"),
	}
	// The fake Gmail server always reports the same historyId and message,
	// simulating Gmail resending an overlapping history window on the next
	// poll -- the reader's own in-memory dedupe must catch this, not the
	// (fixed) server response.
	gmailSrv := newGmailServer(t, "200", []string{"m1"}, rawByID)
	defer gmailSrv.Close()
	tokenSrv := newTokenServer(t, "at-1", false)
	defer tokenSrv.Close()
	rec := &forwardRecorder{}
	fwdSrv := rec.server()
	defer fwdSrv.Close()

	store := &fakeStore{accounts: []OAuthAccount{
		{ID: 1, CompanyID: 7, ExternalID: own, RefreshEnc: encryptedRefreshToken(t, "rt-1"), HistoryID: "100"},
	}}
	r := newTestReader(t, store, tokenSrv, gmailSrv, fwdSrv.URL)

	r.pollOnce(context.Background())
	r.pollOnce(context.Background())

	if got := rec.count(); got != 1 {
		t.Fatalf("forwarded %d messages across two polls, want 1 (dedupe failed)", got)
	}
}

func TestPollAccountInvalidGrantMarksNeedsReconnect(t *testing.T) {
	own := "tenant@gmail.com"
	gmailSrv := newGmailServer(t, "200", nil, nil)
	defer gmailSrv.Close()
	tokenSrv := newTokenServer(t, "", true)
	defer tokenSrv.Close()
	rec := &forwardRecorder{}
	fwdSrv := rec.server()
	defer fwdSrv.Close()

	store := &fakeStore{accounts: []OAuthAccount{
		{ID: 1, CompanyID: 7, ExternalID: own, RefreshEnc: encryptedRefreshToken(t, "rt-revoked"), HistoryID: "100"},
	}}
	r := newTestReader(t, store, tokenSrv, gmailSrv, fwdSrv.URL)

	r.pollOnce(context.Background())

	if got := store.stateOf(1); got != OAuthStateNeedsReconnect {
		t.Fatalf("oauth state = %q, want %q", got, OAuthStateNeedsReconnect)
	}
	if got := rec.count(); got != 0 {
		t.Fatalf("forwarded %d messages, want 0 when the refresh token is rejected", got)
	}
	if got := store.historyOf(1); got != "" {
		t.Fatalf("historyId should not be saved when the account never reached history fetch, got %q", got)
	}
}

func TestPollAccountContinuesAfterOneAccountFails(t *testing.T) {
	own := "tenant@gmail.com"
	rawByID := map[string][]byte{
		"m1": rawMessage("customer@example.com", own, "hello", "body"),
	}
	gmailSrv := newGmailServer(t, "200", []string{"m1"}, rawByID)
	defer gmailSrv.Close()
	tokenSrv := newTokenServer(t, "at-1", false)
	defer tokenSrv.Close()
	rec := &forwardRecorder{}
	fwdSrv := rec.server()
	defer fwdSrv.Close()

	store := &fakeStore{accounts: []OAuthAccount{
		{ID: 1, CompanyID: 7, ExternalID: "broken@gmail.com", RefreshEnc: "not-valid-ciphertext", HistoryID: "100"},
		{ID: 2, CompanyID: 8, ExternalID: own, RefreshEnc: encryptedRefreshToken(t, "rt-1"), HistoryID: "100"},
	}}
	r := newTestReader(t, store, tokenSrv, gmailSrv, fwdSrv.URL)

	r.pollOnce(context.Background())

	if got := rec.count(); got != 1 {
		t.Fatalf("forwarded %d messages, want 1 (account 2 must still be polled after account 1 fails to decrypt)", got)
	}
}

func TestBuildPayloadForcesOwnAddressIntoRecipients(t *testing.T) {
	raw := rawMessage("customer@example.com", "alias@example.com", "hi", "body text")
	p, err := buildPayload(raw, "tenant@gmail.com", 8*1024*1024)
	if err != nil {
		t.Fatalf("buildPayload: %v", err)
	}
	if len(p.Recipients) == 0 || p.Recipients[0] != "tenant@gmail.com" {
		t.Fatalf("Recipients = %v, want tenant@gmail.com first", p.Recipients)
	}
	if p.From != "customer@example.com" {
		t.Fatalf("From = %q, want customer@example.com", p.From)
	}
	if p.Body != "body text" {
		t.Fatalf("Body = %q, want %q", p.Body, "body text")
	}
}

func TestIsSelfSentMatchesOwnAndSystemMailbox(t *testing.T) {
	if !isSelfSent("Tenant@Gmail.com", "tenant@gmail.com", "") {
		t.Fatal("expected case-insensitive match against own address")
	}
	if !isSelfSent("system@mail.internal", "tenant@gmail.com", "system@mail.internal") {
		t.Fatal("expected match against system mailbox")
	}
	if isSelfSent("customer@example.com", "tenant@gmail.com", "system@mail.internal") {
		t.Fatal("expected no match for a genuine customer sender")
	}
}
