package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type OllamaConfig struct {
	BaseURL    string
	Model      string
	Dimensions int
}

type OllamaProvider struct {
	baseURL    string
	model      string
	dimensions int
	client     *http.Client
}

func NewOllamaProvider(cfg OllamaConfig) *OllamaProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "all-minilm"
	}
	dimensions := cfg.Dimensions
	if dimensions <= 0 {
		dimensions = ollamaDefaultDimensions(model)
	}
	return &OllamaProvider{
		baseURL:    baseURL,
		model:      model,
		dimensions: dimensions,
		client:     &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OllamaProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body := map[string]any{
		"input": texts,
		"model": p.model,
	}
	if p.dimensions > 0 {
		body["dimensions"] = p.dimensions
	}

	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(body); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(), &payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Error != "" {
			return nil, fmt.Errorf("ollama embeddings returned %s: %s", resp.Status, errBody.Error)
		}
		return nil, fmt.Errorf("ollama embeddings returned %s", resp.Status)
	}

	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("embedding count %d does not match input count %d", len(out.Embeddings), len(texts))
	}
	return out.Embeddings, nil
}

func (p *OllamaProvider) Dim() int {
	return p.dimensions
}

func (p *OllamaProvider) endpoint() string {
	if strings.HasSuffix(p.baseURL, "/api") {
		return p.baseURL + "/embed"
	}
	return p.baseURL + "/api/embed"
}

func ollamaDefaultDimensions(model string) int {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "all-minilm":
		return 384
	case "nomic-embed-text":
		return 768
	case "mxbai-embed-large":
		return 1024
	default:
		return 384
	}
}
