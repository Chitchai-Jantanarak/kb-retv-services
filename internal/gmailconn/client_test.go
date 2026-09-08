package gmailconn

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"context"
)

func TestHistoryPaginates(t *testing.T) {
	pages := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/users/me/history", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer at-1" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.URL.Query().Get("startHistoryId"); got != "100" {
			t.Fatalf("startHistoryId = %q, want 100", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "" {
			pages++
			w.Write([]byte(`{
				"history": [{"messagesAdded": [{"message": {"id": "m1"}}]}],
				"nextPageToken": "page2"
			}`))
			return
		}
		pages++
		w.Write([]byte(`{
			"history": [{"messagesAdded": [{"message": {"id": "m2"}}]}],
			"historyId": "150"
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(srv.URL, srv.Client())
	ids, newHistoryID, err := c.History(context.Background(), "at-1", "100")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if pages != 2 {
		t.Fatalf("expected 2 pages fetched, got %d", pages)
	}
	if len(ids) != 2 || ids[0] != "m1" || ids[1] != "m2" {
		t.Fatalf("ids = %v, want [m1 m2]", ids)
	}
	if newHistoryID != "150" {
		t.Fatalf("newHistoryID = %q, want 150", newHistoryID)
	}
}

func TestHistory404FallsBackToRecentList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users/me/history", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/users/me/messages", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "newer_than:1d" {
			t.Fatalf("q = %q, want newer_than:1d", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"messages":[{"id":"fallback-1"},{"id":"fallback-2"}]}`))
	})
	mux.HandleFunc("/users/me/profile", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"historyId":"999","emailAddress":"a@b.com"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(srv.URL, srv.Client())
	ids, newHistoryID, err := c.History(context.Background(), "at-1", "1")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(ids) != 2 || ids[0] != "fallback-1" || ids[1] != "fallback-2" {
		t.Fatalf("ids = %v, want [fallback-1 fallback-2]", ids)
	}
	if newHistoryID != "999" {
		t.Fatalf("newHistoryID = %q, want 999", newHistoryID)
	}
}

func TestHistoryEmptyStartUsesFallbackDirectly(t *testing.T) {
	historyHit := false
	mux := http.NewServeMux()
	mux.HandleFunc("/users/me/history", func(w http.ResponseWriter, r *http.Request) {
		historyHit = true
	})
	mux.HandleFunc("/users/me/messages", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"messages":[]}`))
	})
	mux.HandleFunc("/users/me/profile", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"historyId":"1"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(srv.URL, srv.Client())
	if _, _, err := c.History(context.Background(), "at-1", ""); err != nil {
		t.Fatalf("History: %v", err)
	}
	if historyHit {
		t.Fatal("expected no call to /users/me/history when startHistoryId is empty")
	}
}

func TestRawDecodesBase64URL(t *testing.T) {
	raw := []byte("From: a@b.com\r\nSubject: hi\r\n\r\nbody")
	encoded := base64.RawURLEncoding.EncodeToString(raw)

	mux := http.NewServeMux()
	mux.HandleFunc("/users/me/messages/m1", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("format"); got != "raw" {
			t.Fatalf("format = %q, want raw", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"m1","raw":"` + encoded + `"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(srv.URL, srv.Client())
	got, err := c.Raw(context.Background(), "at-1", "m1")
	if err != nil {
		t.Fatalf("Raw: %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("raw = %q, want %q", got, raw)
	}
}

func TestRawNon200ReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users/me/messages/m1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(srv.URL, srv.Client())
	if _, err := c.Raw(context.Background(), "at-1", "m1"); err == nil {
		t.Fatal("expected error on 403")
	}
}
