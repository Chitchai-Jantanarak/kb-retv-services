package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

type VoyageConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	Dimensions int
	InputType  string
}

type VoyageProvider struct {
	baseURL    string
	apiKey     string
	model      string
	dimensions int
	inputType  string
	client     *http.Client
}

func NewVoyageProvider(cfg VoyageConfig) *VoyageProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.voyageai.com/v1"
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "voyage-4"
	}
	dimensions := cfg.Dimensions
	if dimensions <= 0 {
		dimensions = 1024
	}
	inputType := strings.TrimSpace(cfg.InputType)
	if inputType == "" {
		inputType = "document"
	}
	return &VoyageProvider{
		baseURL:    baseURL,
		apiKey:     cfg.APIKey,
		model:      model,
		dimensions: dimensions,
		inputType:  inputType,
		client:     &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *VoyageProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if strings.TrimSpace(p.apiKey) == "" {
		return nil, errors.New("voyage api key is required")
	}
	if len(texts) == 0 {
		return nil, nil
	}

	body := map[string]any{
		"input":      texts,
		"model":      p.model,
		"input_type": p.inputType,
	}
	if p.dimensions > 0 {
		body["output_dimension"] = p.dimensions
	}

	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(body); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/embeddings", &payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error any `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Error != nil {
			return nil, fmt.Errorf("voyage embeddings returned %s: %v", resp.Status, errBody.Error)
		}
		return nil, fmt.Errorf("voyage embeddings returned %s", resp.Status)
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

func (p *VoyageProvider) Dim() int {
	return p.dimensions
}
