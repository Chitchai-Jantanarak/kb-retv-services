package llm

import (
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/anthropic"
	"github.com/my/app/internal/infra/llm/gemini"
	"github.com/my/app/internal/infra/llm/openai"
)

const (
	VendorOpenAI     = "openai"
	VendorAnthropic  = "anthropic"
	VendorClaude     = "claude"
	VendorGemini     = "gemini"
	VendorOpenRouter = "openrouter"

	openRouterBaseURL = "https://openrouter.ai/api/v1"
)

// Settings is the union of every provider's config. Resolve picks the
// fields it needs based on Vendor. APIKey fallbacks are evaluated in order:
// vendor-specific key, then OpenRouterKey when vendor is openrouter.
type Settings struct {
	Vendor          string
	Model           string
	OpenAIKey       string
	AnthropicKey    string
	GeminiKey       string
	OpenRouterKey   string
	OpenAIBaseURL   string
	OpenRouterTitle string
	ReferrerURL     string
}

// Resolve returns an LLMProvider for the requested vendor.
// Vendor "claude" is an alias for "anthropic".
// Vendor "openrouter" reuses the OpenAI client pointed at OpenRouter's
// OpenAI-compatible endpoint; pass the OpenRouter model slug in Settings.Model
// (e.g. "google/gemini-2.0-flash-exp:free", "anthropic/claude-haiku-4.5").
func Resolve(s Settings) (ports.LLMProvider, error) {
	vendor := strings.ToLower(strings.TrimSpace(s.Vendor))
	switch vendor {
	case VendorOpenAI:
		return openai.New(openai.Config{
			APIKey:      s.OpenAIKey,
			BaseURL:     s.OpenAIBaseURL,
			Model:       s.Model,
			ReferrerURL: s.ReferrerURL,
		})
	case VendorOpenRouter:
		if s.OpenRouterKey == "" {
			return nil, errors.New("llm: openrouter requires APIKeys.OpenRouter")
		}
		return openai.New(openai.Config{
			APIKey:      s.OpenRouterKey,
			BaseURL:     openRouterBaseURL,
			Model:       s.Model,
			ReferrerURL: s.ReferrerURL,
			Title:       s.OpenRouterTitle,
		})
	case VendorAnthropic, VendorClaude:
		return anthropic.New(anthropic.Config{
			APIKey: s.AnthropicKey,
			Model:  s.Model,
		})
	case VendorGemini:
		return gemini.New(gemini.Config{
			APIKey: s.GeminiKey,
			Model:  s.Model,
		})
	case "":
		return nil, errors.New("llm: vendor is required")
	default:
		return nil, fmt.Errorf("llm: unsupported vendor %q", vendor)
	}
}
