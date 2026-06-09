package embeddings

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type OllamaConfig struct {
	BaseURL        string
	Model          string
	Dimensions     int
	MaxRetries     int
	RequestsPerMin float64
}

type OllamaProvider struct {
	baseURL    string
	model      string
	dimensions int
	retrier    *httpRetrier
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
		retrier:    newHTTPRetrier(cfg.MaxRetries, cfg.RequestsPerMin, 120*time.Second),
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

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	resp, err := p.retrier.post(ctx, p.endpoint(), payload, map[string]string{
		"Content-Type": "application/json",
	})
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
