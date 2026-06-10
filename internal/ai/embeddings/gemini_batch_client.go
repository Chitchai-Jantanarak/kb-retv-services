package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type GeminiBatchClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewGeminiBatchClient(cfg GeminiConfig) *GeminiBatchClient {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	return &GeminiBatchClient{
		baseURL: baseURL,
		apiKey:  cfg.APIKey,
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

type BatchJob struct {
	Name           string
	State          string
	ResultFileName string
}

func (j BatchJob) Succeeded() bool {
	return strings.Contains(j.State, "SUCCEEDED")
}

func (j BatchJob) Done() bool {
	for _, terminal := range []string{"SUCCEEDED", "FAILED", "CANCELLED", "EXPIRED"} {
		if strings.Contains(j.State, terminal) {
			return true
		}
	}
	return false
}

func (c *GeminiBatchClient) root() string {
	return strings.TrimSuffix(c.baseURL, "/v1beta")
}

func (c *GeminiBatchClient) UploadJSONL(ctx context.Context, displayName string, data []byte) (string, error) {
	startBody, err := json.Marshal(map[string]any{"file": map[string]any{"display_name": displayName}})
	if err != nil {
		return "", err
	}

	startReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.root()+"/upload/v1beta/files", bytes.NewReader(startBody))
	if err != nil {
		return "", err
	}
	startReq.Header.Set("x-goog-api-key", c.apiKey)
	startReq.Header.Set("X-Goog-Upload-Protocol", "resumable")
	startReq.Header.Set("X-Goog-Upload-Command", "start")
	startReq.Header.Set("X-Goog-Upload-Header-Content-Length", strconv.Itoa(len(data)))
	startReq.Header.Set("X-Goog-Upload-Header-Content-Type", "application/jsonl")
	startReq.Header.Set("Content-Type", "application/json")

	startResp, err := c.client.Do(startReq)
	if err != nil {
		return "", err
	}
	defer drainClose(startResp.Body)
	if startResp.StatusCode < 200 || startResp.StatusCode >= 300 {
		return "", fmt.Errorf("gemini batch upload start returned %s", startResp.Status)
	}
	uploadURL := startResp.Header.Get("X-Goog-Upload-URL")
	if uploadURL == "" {
		return "", fmt.Errorf("gemini batch upload: missing X-Goog-Upload-URL header")
	}

	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	uploadReq.Header.Set("x-goog-api-key", c.apiKey)
	uploadReq.Header.Set("X-Goog-Upload-Command", "upload, finalize")
	uploadReq.Header.Set("X-Goog-Upload-Offset", "0")
	uploadReq.Header.Set("Content-Type", "application/jsonl")

	uploadResp, err := c.client.Do(uploadReq)
	if err != nil {
		return "", err
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode < 200 || uploadResp.StatusCode >= 300 {
		return "", fmt.Errorf("gemini batch upload returned %s", uploadResp.Status)
	}

	var out struct {
		File struct {
			Name string `json:"name"`
		} `json:"file"`
	}
	if err := json.NewDecoder(uploadResp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.File.Name == "" {
		return "", fmt.Errorf("gemini batch upload: response missing file name")
	}
	return out.File.Name, nil
}

func (c *GeminiBatchClient) CreateEmbedBatch(ctx context.Context, model, displayName, fileName string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"batch": map[string]any{
			"display_name": displayName,
			"input_config": map[string]any{"file_name": fileName},
		},
	})
	if err != nil {
		return "", err
	}

	url := c.baseURL + "/models/" + model + ":asyncBatchEmbedContent"

	const maxRetries = 3
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := c.post(ctx, url, body)
		if err != nil {
			return "", err
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			snippet := readSnippet(resp.Body)
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("gemini batch create returned %s: %s", resp.Status, snippet)
			if attempt == maxRetries {
				return "", lastErr
			}
			if werr := sleepBackoff(ctx, attempt, 0); werr != nil {
				return "", werr
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			snippet := readSnippet(resp.Body)
			_ = resp.Body.Close()
			return "", fmt.Errorf("gemini batch create returned %s: %s", resp.Status, snippet)
		}

		var out struct {
			Name string `json:"name"`
		}
		err = json.NewDecoder(resp.Body).Decode(&out)
		_ = resp.Body.Close()
		if err != nil {
			return "", err
		}
		if out.Name == "" {
			return "", fmt.Errorf("gemini batch create: response missing name")
		}
		return out.Name, nil
	}
	return "", lastErr
}

func (c *GeminiBatchClient) GetBatch(ctx context.Context, name string) (BatchJob, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/"+name, nil)
	if err != nil {
		return BatchJob{}, err
	}
	req.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return BatchJob{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BatchJob{}, fmt.Errorf("gemini batch get returned %s: %s", resp.Status, readSnippet(resp.Body))
	}

	var body struct {
		Name     string `json:"name"`
		State    string `json:"state"`
		Metadata struct {
			State  string `json:"state"`
			Output struct {
				ResponsesFile string `json:"responsesFile"`
			} `json:"output"`
		} `json:"metadata"`
		Response struct {
			ResponsesFile string `json:"responsesFile"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return BatchJob{}, err
	}

	return BatchJob{
		Name:           firstProviderNonEmpty(body.Name, name),
		State:          firstProviderNonEmpty(body.Metadata.State, body.State),
		ResultFileName: firstProviderNonEmpty(body.Metadata.Output.ResponsesFile, body.Response.ResponsesFile),
	}, nil
}

func (c *GeminiBatchClient) DownloadResults(ctx context.Context, resultFileName string) ([]byte, error) {
	url := c.root() + "/download/v1beta/" + resultFileName + ":download?alt=media"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini batch download returned %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func (c *GeminiBatchClient) post(ctx context.Context, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	return c.client.Do(req)
}

func drainClose(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}

func readSnippet(body io.Reader) string {
	raw, _ := io.ReadAll(io.LimitReader(body, 4096))
	return strings.TrimSpace(string(raw))
}
