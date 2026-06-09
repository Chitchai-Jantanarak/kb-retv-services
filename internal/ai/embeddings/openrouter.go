package embeddings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type OpenRouterConfig struct {
	BaseURL        string
	APIKey         string
	Model          string
	Dimensions     int
	MaxRetries     int
	RequestsPerMin float64
}

type OpenRouterProvider struct {
	baseURL    string
	apiKey     string
	model      string
	dimensions int
	retrier    *httpRetrier
}

func NewOpenRouterProvider(cfg OpenRouterConfig) *OpenRouterProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "openai/text-embedding-3-small"
	}
	dimensions := cfg.Dimensions
	if dimensions <= 0 {
		dimensions = defaultDimensions(strings.TrimPrefix(model, "openai/"))
	}
	return &OpenRouterProvider{
		baseURL:    baseURL,
		apiKey:     cfg.APIKey,
		model:      model,
		dimensions: dimensions,
		retrier:    newHTTPRetrier(cfg.MaxRetries, cfg.RequestsPerMin, 60*time.Second),
	}
}

func (p *OpenRouterProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if strings.TrimSpace(p.apiKey) == "" {
		return nil, errors.New("openrouter api key is required")
	}
	if len(texts) == 0 {
		return nil, nil
	}

	body := map[string]any{
		"input": texts,
		"model": p.model,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	resp, err := p.retrier.post(ctx, p.baseURL+"/embeddings", payload, map[string]string{
		"Authorization": "Bearer " + p.apiKey,
		"Content-Type":  "application/json",
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Error.Message != "" {
			return nil, fmt.Errorf("openrouter embeddings returned %s: %s", resp.Status, errBody.Error.Message)
		}
		return nil, fmt.Errorf("openrouter embeddings returned %s", resp.Status)
	}

	var out struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("embedding count %d does not match input count %d", len(out.Data), len(texts))
	}

	sort.Slice(out.Data, func(i int, j int) bool {
		return out.Data[i].Index < out.Data[j].Index
	})

	vectors := make([][]float32, 0, len(out.Data))
	for _, item := range out.Data {
		vectors = append(vectors, item.Embedding)
	}
	return vectors, nil
}

func (p *OpenRouterProvider) Dim() int {
	return p.dimensions
}
