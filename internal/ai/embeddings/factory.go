package embeddings

import (
	"fmt"
	"strings"

	"github.com/my/app/internal/domain/ports"
)

type ProviderSettings struct {
	Provider       string
	Model          string
	BaseURL        string
	Dimensions     int
	OpenAIKey      string
	GeminiKey      string
	VoyageKey      string
	OpenRouterKey  string
	LocalURL       string
	MaxRetries     int
	RequestsPerMin float64
}

func NewProvider(settings ProviderSettings) (string, string, ports.EmbeddingProvider, error) {
	provider := ResolveProvider(settings)
	model := DefaultModel(provider, settings.Model)
	switch provider {
	case "openai":
		return provider, model, NewOpenAIProvider(OpenAIConfig{
			BaseURL:        settings.BaseURL,
			APIKey:         settings.OpenAIKey,
			Model:          model,
			Dimensions:     settings.Dimensions,
			MaxRetries:     settings.MaxRetries,
			RequestsPerMin: settings.RequestsPerMin,
		}), nil
	case "gemini":
		return provider, model, NewGeminiProvider(GeminiConfig{
			BaseURL:        settings.BaseURL,
			APIKey:         settings.GeminiKey,
			Model:          model,
			Dimensions:     settings.Dimensions,
			MaxRetries:     settings.MaxRetries,
			RequestsPerMin: settings.RequestsPerMin,
		}), nil
	case "openrouter":
		return provider, model, NewOpenRouterProvider(OpenRouterConfig{
			BaseURL:        settings.BaseURL,
			APIKey:         settings.OpenRouterKey,
			Model:          model,
			Dimensions:     settings.Dimensions,
			MaxRetries:     settings.MaxRetries,
			RequestsPerMin: settings.RequestsPerMin,
		}), nil
	case "ollama":
		return provider, model, NewOllamaProvider(OllamaConfig{
			BaseURL:        firstProviderNonEmpty(settings.BaseURL, settings.LocalURL),
			Model:          model,
			Dimensions:     settings.Dimensions,
			MaxRetries:     settings.MaxRetries,
			RequestsPerMin: settings.RequestsPerMin,
		}), nil
	case "voyage", "claude", "anthropic":
		return provider, model, NewVoyageProvider(VoyageConfig{
			BaseURL:        settings.BaseURL,
			APIKey:         settings.VoyageKey,
			Model:          model,
			Dimensions:     settings.Dimensions,
			MaxRetries:     settings.MaxRetries,
			RequestsPerMin: settings.RequestsPerMin,
		}), nil
	case "hash":
		dim := settings.Dimensions
		if dim <= 0 {
			dim = 384
		}
		return provider, model, NewHashProvider(dim), nil
	default:
		return "", "", nil, fmt.Errorf("unknown embedding provider %q", settings.Provider)
	}
}

func ResolveProvider(settings ProviderSettings) string {
	provider := strings.ToLower(strings.TrimSpace(settings.Provider))
	if provider != "" && provider != "auto" {
		return provider
	}
	switch {
	case strings.TrimSpace(settings.OpenRouterKey) != "":
		return "openrouter"
	case strings.TrimSpace(settings.OpenAIKey) != "":
		return "openai"
	case strings.TrimSpace(settings.GeminiKey) != "":
		return "gemini"
	case strings.TrimSpace(settings.VoyageKey) != "":
		return "voyage"
	case strings.TrimSpace(settings.LocalURL) != "":
		return "ollama"
	default:
		return "hash"
	}
}

func DefaultModel(provider string, model string) string {
	if strings.TrimSpace(model) != "" {
		return model
	}
	switch provider {
	case "openai":
		return "text-embedding-3-small"
	case "gemini":
		return "gemini-embedding-2"
	case "openrouter":
		return "nvidia/llama-nemotron-embed-vl-1b-v2:free"
	case "ollama":
		return "all-minilm"
	case "voyage", "claude", "anthropic":
		return "voyage-4"
	default:
		return "hash-dev"
	}
}

func firstProviderNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
