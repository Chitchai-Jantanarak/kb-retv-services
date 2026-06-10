package embeddings

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newBatchClient(server *httptest.Server) *GeminiBatchClient {
	return NewGeminiBatchClient(GeminiConfig{
		BaseURL: server.URL + "/v1beta",
		APIKey:  "test-key",
	})
}

func TestBatchClientUploadJSONLResumable(t *testing.T) {
	var startSeen, uploadSeen bool
	var uploadedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Header.Get("X-Goog-Upload-Command") == "start":
			startSeen = true
			if r.Header.Get("x-goog-api-key") != "test-key" {
				t.Fatalf("missing api key header")
			}
			w.Header().Set("X-Goog-Upload-URL", "http://"+r.Host+"/upload/part")
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.Header.Get("X-Goog-Upload-Command"), "upload"):
			uploadSeen = true
			body, _ := io.ReadAll(r.Body)
			uploadedBody = string(body)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"file":{"name":"files/abc123"}}`))
		default:
			t.Fatalf("unexpected upload command %q", r.Header.Get("X-Goog-Upload-Command"))
		}
	}))
	defer server.Close()

	fileName, err := newBatchClient(server).UploadJSONL(context.Background(), "batch-embeddings", []byte("line1\nline2\n"))
	if err != nil {
		t.Fatalf("UploadJSONL error = %v", err)
	}
	if !startSeen || !uploadSeen {
		t.Fatalf("resumable steps: start=%v upload=%v", startSeen, uploadSeen)
	}
	if uploadedBody != "line1\nline2\n" {
		t.Fatalf("uploaded body = %q", uploadedBody)
	}
	if fileName != "files/abc123" {
		t.Fatalf("fileName = %q, want files/abc123", fileName)
	}
}

func TestBatchClientCreateEmbedBatch(t *testing.T) {
	var path string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"batches/job-789"}`))
	}))
	defer server.Close()

	name, err := newBatchClient(server).CreateEmbedBatch(context.Background(), "gemini-embedding-2", "kb-global", "files/abc123")
	if err != nil {
		t.Fatalf("CreateEmbedBatch error = %v", err)
	}
	if !strings.HasSuffix(path, "/models/gemini-embedding-2:asyncBatchEmbedContent") {
		t.Fatalf("path = %q, want .../models/gemini-embedding-2:asyncBatchEmbedContent", path)
	}
	batch := body["batch"].(map[string]any)
	if batch["display_name"] != "kb-global" {
		t.Fatalf("display_name = %v, want kb-global", batch["display_name"])
	}
	input := batch["input_config"].(map[string]any)
	if input["file_name"] != "files/abc123" {
		t.Fatalf("file_name = %v, want files/abc123", input["file_name"])
	}
	if name != "batches/job-789" {
		t.Fatalf("name = %q, want batches/job-789", name)
	}
}

func TestBatchClientGetBatchSucceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/batches/job-789") {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"name":"batches/job-789",
			"metadata":{"state":"JOB_STATE_SUCCEEDED","output":{"responsesFile":"files/results-1"}}
		}`))
	}))
	defer server.Close()

	job, err := newBatchClient(server).GetBatch(context.Background(), "batches/job-789")
	if err != nil {
		t.Fatalf("GetBatch error = %v", err)
	}
	if !job.Succeeded() {
		t.Fatalf("job.Succeeded() = false, state = %q", job.State)
	}
	if job.ResultFileName != "files/results-1" {
		t.Fatalf("ResultFileName = %q, want files/results-1", job.ResultFileName)
	}
}

func TestBatchClientGetBatchRunning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name":"batches/job-789","metadata":{"state":"JOB_STATE_RUNNING"}}`))
	}))
	defer server.Close()

	job, err := newBatchClient(server).GetBatch(context.Background(), "batches/job-789")
	if err != nil {
		t.Fatalf("GetBatch error = %v", err)
	}
	if job.Succeeded() {
		t.Fatal("running job should not report Succeeded")
	}
	if job.Done() {
		t.Fatal("running job should not report Done")
	}
}

func TestBatchClientGetBatchFailedIsDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"name":"batches/job-789","metadata":{"state":"JOB_STATE_FAILED"}}`))
	}))
	defer server.Close()

	job, err := newBatchClient(server).GetBatch(context.Background(), "batches/job-789")
	if err != nil {
		t.Fatalf("GetBatch error = %v", err)
	}
	if job.Succeeded() {
		t.Fatal("failed job is not succeeded")
	}
	if !job.Done() {
		t.Fatal("failed job should report Done (terminal)")
	}
}

func TestBatchClientDownloadResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/download/v1beta/files/results-1") {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("alt") != "media" {
			t.Fatalf("alt = %q, want media", r.URL.Query().Get("alt"))
		}
		w.Write([]byte(`{"key":"2:10","response":{"embedding":{"values":[0.1]}}}` + "\n"))
	}))
	defer server.Close()

	data, err := newBatchClient(server).DownloadResults(context.Background(), "files/results-1")
	if err != nil {
		t.Fatalf("DownloadResults error = %v", err)
	}
	if !strings.Contains(string(data), `"key":"2:10"`) {
		t.Fatalf("downloaded = %q", data)
	}
}
