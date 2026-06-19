package local

import (
	"strings"

	"github.com/my/app/internal/infra/llm/openai"
)

const (
	DefaultBaseURL = "http://localhost:11434/v1"
	sentinelKey    = "local"
)

type Config struct {
	APIKey  string
	BaseURL string
	Model   string
}

func New(cfg Config) (*openai.Client, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	key := strings.TrimSpace(cfg.APIKey)
	if key == "" {
		key = sentinelKey
	}
	return openai.New(openai.Config{
		APIKey:  key,
		BaseURL: baseURL,
		Model:   cfg.Model,
	})
}
