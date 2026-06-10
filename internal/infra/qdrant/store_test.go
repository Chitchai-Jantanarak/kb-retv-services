package qdrant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/my/app/internal/domain/ports"
)

func TestStoreEnsureCollectionAndUpsertUseHTTPAPI(t *testing.T) {
	var ensureCalled bool
	var upsertCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/collections/kb__3":
			ensureCalled = true
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode ensure body: %v", err)
			}
			vectors := body["vectors"].(map[string]any)
			if vectors["size"].(float64) != 3 {
				t.Fatalf("vector size = %v, want 3", vectors["size"])
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"result":true}`))
		case r.Method == http.MethodPut && r.URL.Path == "/collections/kb__3/points":
			upsertCalled = true
			var body map[string][]map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode upsert body: %v", err)
			}
			points := body["points"]
			if len(points) != 1 {
				t.Fatalf("points = %d, want 1", len(points))
			}
			if points[0]["id"] != float64(1) {
				t.Fatalf("point id = %v, want 1", points[0]["id"])
			}
			payload := points[0]["payload"].(map[string]any)
			if payload["company_id"] != float64(3) {
				t.Fatalf("company_id payload = %v, want 3", payload["company_id"])
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"result":true}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	store := NewStore(Config{URL: server.URL})
	if err := store.EnsureCollection(context.Background(), "kb__3", 3); err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}
	if err := store.Upsert(context.Background(), "kb__3", []ports.VectorPoint{
		{ID: "1", Vector: []float32{1, 2, 3}, Metadata: map[string]any{"company_id": int64(3)}},
	}); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	if !ensureCalled {
		t.Fatal("ensure endpoint was not called")
	}
	if !upsertCalled {
		t.Fatal("upsert endpoint was not called")
	}
}

func TestStoreEnsureCollectionAllowsExistingCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/collections/kb__3" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"status":{"error":"Wrong input: Collection kb__3 already exists!"}}`))
	}))
	defer server.Close()

	store := NewStore(Config{URL: server.URL})
	if err := store.EnsureCollection(context.Background(), "kb__3", 3); err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}
}

func TestStoreUpsertSplitsIntoBatchesOf200(t *testing.T) {
	var putCalls int
	var seen int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/collections/kb__3/points" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		putCalls++
		var body map[string][]map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode upsert body: %v", err)
		}
		batch := len(body["points"])
		if batch > 200 {
			t.Fatalf("batch size = %d, want <= 200", batch)
		}
		seen += batch
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":true}`))
	}))
	defer server.Close()

	points := make([]ports.VectorPoint, 450)
	for i := range points {
		points[i] = ports.VectorPoint{ID: strconv.Itoa(i + 1), Vector: []float32{1, 2, 3}}
	}

	store := NewStore(Config{URL: server.URL})
	if err := store.Upsert(context.Background(), "kb__3", points); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if putCalls != 3 {
		t.Fatalf("PUT calls = %d, want 3 (200+200+50)", putCalls)
	}
	if seen != 450 {
		t.Fatalf("points received = %d, want 450", seen)
	}
}

func TestStoreUpsertErrorIncludesResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"status":{"error":"bad vector dimension"}}`))
	}))
	defer server.Close()

	store := NewStore(Config{URL: server.URL})
	err := store.Upsert(context.Background(), "kb__3", []ports.VectorPoint{
		{ID: "1", Vector: []float32{1, 2, 3}},
	})
	if err == nil {
		t.Fatal("Upsert() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "bad vector dimension") {
		t.Fatalf("error %q should include the response body snippet", err.Error())
	}
}

func TestStoreRejectsUnscopedCollectionNames(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := NewStore(Config{URL: server.URL})
	ctx := context.Background()
	cases := []struct {
		name string
		run  func() error
	}{
		{name: "ensure", run: func() error { return store.EnsureCollection(ctx, "kb", 3) }},
		{name: "upsert", run: func() error { return store.Upsert(ctx, "kb", nil) }},
		{name: "search", run: func() error {
			_, err := store.Search(ctx, ports.VectorSearch{Collection: "kb", Query: []float32{1}, Limit: 1})
			return err
		}},
		{name: "delete", run: func() error { return store.Delete(ctx, "kb", []string{"1"}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			err := tc.run()
			if err == nil {
				t.Fatal("err = nil, want invalid collection error")
			}
			if called {
				t.Fatal("http server was called for invalid collection")
			}
		})
	}
}
