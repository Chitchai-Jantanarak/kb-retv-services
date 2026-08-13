package embeddings

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"sort"
	"strings"
)

type openaiCompatProvider struct {
	baseURL     string
	apiKey      string
	model       string
	dimensions  int
	retrier     *httpRetrier
	vendor      string
	extraFields func() map[string]any
	formatError func(status string, body io.Reader) error
}

func (p *openaiCompatProvider) embed(ctx context.Context, texts []string) ([][]float32, error) {
	if strings.TrimSpace(p.apiKey) == "" {
		return nil, fmt.Errorf("%s api key is required", p.vendor)
	}
	if len(texts) == 0 {
		return nil, nil
	}

	body := map[string]any{
		"input": texts,
		"model": p.model,
	}
	if p.extraFields != nil {
		maps.Copy(body, p.extraFields())
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
		return nil, p.formatError(resp.Status, resp.Body)
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

func (p *openaiCompatProvider) Dim() int {
	return p.dimensions
}

func openAICompatErrorMessage(vendor string) func(status string, body io.Reader) error {
	return func(status string, body io.Reader) error {
		var errBody struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(body).Decode(&errBody)
		if errBody.Error.Message != "" {
			return fmt.Errorf("%s embeddings returned %s: %s", vendor, status, errBody.Error.Message)
		}
		return fmt.Errorf("%s embeddings returned %s", vendor, status)
	}
}

func defaultDimensions(model string) int {
	switch model {
	case "text-embedding-3-large":
		return 3072
	default:
		return 1536
	}
}
