package embeddings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type GeminiConfig struct {
	BaseURL        string
	APIKey         string
	Model          string
	Dimensions     int
	MaxRetries     int
	RequestsPerMin float64
}

type GeminiProvider struct {
	baseURL    string
	apiKey     string
	model      string
	dimensions int
	retrier    *httpRetrier
}

func NewGeminiProvider(cfg GeminiConfig) *GeminiProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	model := strings.TrimPrefix(strings.TrimSpace(cfg.Model), "models/")
	if model == "" {
		model = "gemini-embedding-2"
	}
	dimensions := cfg.Dimensions
	if dimensions <= 0 {
		dimensions = 3072
	}
	return &GeminiProvider{
		baseURL:    baseURL,
		apiKey:     cfg.APIKey,
		model:      model,
		dimensions: dimensions,
		retrier:    newHTTPRetrier(cfg.MaxRetries, cfg.RequestsPerMin, 60*time.Second),
	}
}

func (p *GeminiProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if strings.TrimSpace(p.apiKey) == "" {
		return nil, errors.New("gemini api key is required")
	}
	if len(texts) == 0 {
		return nil, nil
	}

	requests := make([]map[string]any, 0, len(texts))
	for _, text := range texts {
		request := map[string]any{
			"model": "models/" + p.model,
			"content": map[string]any{
				"parts": []map[string]string{{"text": text}},
			},
		}
		if p.dimensions > 0 {
			request["outputDimensionality"] = p.dimensions
		}
		requests = append(requests, request)
	}

	payload, err := json.Marshal(map[string]any{"requests": requests})
	if err != nil {
		return nil, err
	}

	resp, err := p.retrier.post(ctx, p.baseURL+"/models/"+p.model+":batchEmbedContents", payload, map[string]string{
		"x-goog-api-key": p.apiKey,
		"Content-Type":   "application/json",
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
			return nil, fmt.Errorf("gemini embeddings returned %s: %s", resp.Status, errBody.Error.Message)
		}
		return nil, fmt.Errorf("gemini embeddings returned %s", resp.Status)
	}

	var out struct {
		Embeddings []struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("embedding count %d does not match input count %d", len(out.Embeddings), len(texts))
	}

	vectors := make([][]float32, 0, len(out.Embeddings))
	for _, item := range out.Embeddings {
		vectors = append(vectors, item.Values)
	}
	return vectors, nil
}

func (p *GeminiProvider) Dim() int {
	return p.dimensions
}
