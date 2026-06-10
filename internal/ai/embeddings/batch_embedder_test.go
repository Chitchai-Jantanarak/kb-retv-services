package embeddings

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func geminiBatchTestServer(t *testing.T, results string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Header.Get("X-Goog-Upload-Command") == "start":
			w.Header().Set("X-Goog-Upload-URL", "http://"+r.Host+"/upload/part")
		case strings.Contains(r.Header.Get("X-Goog-Upload-Command"), "upload"):
			w.Write([]byte(`{"file":{"name":"files/in-1"}}`))
		case strings.Contains(r.URL.Path, ":asyncBatchEmbedContent"):
			w.Write([]byte(`{"name":"batches/job-1"}`))
		case strings.Contains(r.URL.Path, "/download/v1beta/"):
			w.Write([]byte(results))
		case strings.Contains(r.URL.Path, "/batches/job-1"):
			w.Write([]byte(`{"name":"batches/job-1","metadata":{"state":"JOB_STATE_SUCCEEDED","output":{"responsesFile":"files/out-1"}}}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
}

func newGeminiBatchEmbedder(server *httptest.Server) *GeminiBatchEmbedder {
	return NewGeminiBatchEmbedder(GeminiConfig{
		BaseURL:    server.URL + "/v1beta",
		APIKey:     "k",
		Model:      "gemini-embedding-2",
		Dimensions: 3072,
	})
}

func TestGeminiBatchEmbedderSubmitReturnsJob(t *testing.T) {
	server := geminiBatchTestServer(t, "")
	defer server.Close()

	embedder := newGeminiBatchEmbedder(server)
	job, err := embedder.Submit(context.Background(), "kb-global", []BatchEmbedRequest{
		{Key: "2:10", Text: "alpha"},
		{Key: "3:20", Text: "beta"},
	})
	if err != nil {
		t.Fatalf("Submit error = %v", err)
	}
	if job != "batches/job-1" {
		t.Fatalf("job = %q, want batches/job-1", job)
	}
	if embedder.Model() != "gemini-embedding-2" || embedder.Dim() != 3072 {
		t.Fatalf("Model/Dim = %q/%d", embedder.Model(), embedder.Dim())
	}
}

func TestGeminiBatchEmbedderPollSucceededReturnsResults(t *testing.T) {
	results := `{"key":"2:10","response":{"embedding":{"values":[0.1,0.2]}}}` + "\n" +
		`{"key":"3:20","response":{"embedding":{"values":[0.3,0.4]}}}` + "\n"
	server := geminiBatchTestServer(t, results)
	defer server.Close()

	poll, err := newGeminiBatchEmbedder(server).Poll(context.Background(), "batches/job-1")
	if err != nil {
		t.Fatalf("Poll error = %v", err)
	}
	if !poll.Succeeded || !poll.Done {
		t.Fatalf("poll = %+v, want done+succeeded", poll)
	}
	if len(poll.Results) != 2 || poll.Results[0].Key != "2:10" {
		t.Fatalf("results = %+v", poll.Results)
	}
}

func TestGeminiBatchEmbedderPollRunningNotDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name":"batches/job-1","metadata":{"state":"JOB_STATE_RUNNING"}}`))
	}))
	defer server.Close()

	poll, err := newGeminiBatchEmbedder(server).Poll(context.Background(), "batches/job-1")
	if err != nil {
		t.Fatalf("Poll error = %v", err)
	}
	if poll.Done || poll.Succeeded {
		t.Fatalf("running poll = %+v, want not done", poll)
	}
	if len(poll.Results) != 0 {
		t.Fatal("running poll should carry no results")
	}
}

func TestGeminiBatchEmbedderPollTerminalFailureErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name":"batches/job-1","metadata":{"state":"JOB_STATE_EXPIRED"}}`))
	}))
	defer server.Close()

	_, err := newGeminiBatchEmbedder(server).Poll(context.Background(), "batches/job-1")
	if err == nil {
		t.Fatal("expected error on terminal non-success state, got nil")
	}
}

func TestNewBatchEmbedderUnsupportedProvider(t *testing.T) {
	_, _, _, err := NewBatchEmbedder(ProviderSettings{Provider: "openai", OpenAIKey: "x"})
	if err == nil {
		t.Fatal("expected error for provider without batch support")
	}
	if !strings.Contains(err.Error(), "openai") {
		t.Fatalf("error %v should name the unsupported provider", err)
	}
}

func TestNewBatchEmbedderGemini(t *testing.T) {
	provider, model, embedder, err := NewBatchEmbedder(ProviderSettings{
		Provider: "gemini", GeminiKey: "k", Model: "gemini-embedding-2", Dimensions: 3072,
	})
	if err != nil {
		t.Fatalf("NewBatchEmbedder error = %v", err)
	}
	if provider != "gemini" || model != "gemini-embedding-2" || embedder == nil {
		t.Fatalf("got provider=%q model=%q embedder=%v", provider, model, embedder)
	}
}
