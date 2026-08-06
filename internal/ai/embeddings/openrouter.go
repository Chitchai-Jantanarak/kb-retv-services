package embeddings

import (
	"context"
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
	compat *openaiCompatProvider
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

	compat := &openaiCompatProvider{
		baseURL:    baseURL,
		apiKey:     cfg.APIKey,
		model:      model,
		dimensions: dimensions,
		retrier:    newHTTPRetrier(cfg.MaxRetries, cfg.RequestsPerMin, 60*time.Second),
		vendor:     "openrouter",
	}
	compat.formatError = openAICompatErrorMessage("openrouter")

	return &OpenRouterProvider{compat: compat}
}

func (p *OpenRouterProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return p.compat.embed(ctx, texts)
}

func (p *OpenRouterProvider) Dim() int {
	return p.compat.Dim()
}
