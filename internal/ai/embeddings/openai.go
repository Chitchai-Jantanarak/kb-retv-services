package embeddings

import (
	"context"
	"strings"
	"time"
)

type OpenAIConfig struct {
	BaseURL        string
	APIKey         string
	Model          string
	Dimensions     int
	MaxRetries     int
	RequestsPerMin float64
}

type OpenAIProvider struct {
	compat *openaiCompatProvider
}

func NewOpenAIProvider(cfg OpenAIConfig) *OpenAIProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "text-embedding-3-small"
	}
	dimensions := cfg.Dimensions
	if dimensions <= 0 {
		dimensions = defaultDimensions(model)
	}

	compat := &openaiCompatProvider{
		baseURL:    baseURL,
		apiKey:     cfg.APIKey,
		model:      model,
		dimensions: dimensions,
		retrier:    newHTTPRetrier(cfg.MaxRetries, cfg.RequestsPerMin, 60*time.Second),
		vendor:     "openai",
	}
	compat.extraFields = func() map[string]any {
		fields := map[string]any{"encoding_format": "float"}
		if compat.dimensions > 0 {
			fields["dimensions"] = compat.dimensions
		}
		return fields
	}
	compat.formatError = openAICompatErrorMessage("openai")

	return &OpenAIProvider{compat: compat}
}

func (p *OpenAIProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return p.compat.embed(ctx, texts)
}

func (p *OpenAIProvider) Dim() int {
	return p.compat.Dim()
}
