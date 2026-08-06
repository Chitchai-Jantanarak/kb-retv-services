package embeddings

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

type VoyageConfig struct {
	BaseURL        string
	APIKey         string
	Model          string
	Dimensions     int
	InputType      string
	MaxRetries     int
	RequestsPerMin float64
}

type VoyageProvider struct {
	compat *openaiCompatProvider
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

	compat := &openaiCompatProvider{
		baseURL:    baseURL,
		apiKey:     cfg.APIKey,
		model:      model,
		dimensions: dimensions,
		retrier:    newHTTPRetrier(cfg.MaxRetries, cfg.RequestsPerMin, 60*time.Second),
		vendor:     "voyage",
	}
	compat.extraFields = func() map[string]any {
		fields := map[string]any{"input_type": inputType}
		if compat.dimensions > 0 {
			fields["output_dimension"] = compat.dimensions
		}
		return fields
	}
	compat.formatError = voyageErrorMessage

	return &VoyageProvider{compat: compat}
}

func (p *VoyageProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return p.compat.embed(ctx, texts)
}

func (p *VoyageProvider) Dim() int {
	return p.compat.Dim()
}

func voyageErrorMessage(status string, body io.Reader) error {
	var errBody struct {
		Error any `json:"error"`
	}
	_ = json.NewDecoder(body).Decode(&errBody)
	if errBody.Error != nil {
		return fmt.Errorf("voyage embeddings returned %s: %v", status, errBody.Error)
	}
	return fmt.Errorf("voyage embeddings returned %s", status)
}
