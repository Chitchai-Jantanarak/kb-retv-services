package llmboot

import (
	"os"
	"time"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/infra/tenant"
	mysqlai "github.com/my/app/internal/repositories/ai/mysql"
	"github.com/my/app/internal/shared/config"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func LLMSettings(cfg config.Config) llm.Settings {
	return llm.Settings{
		Vendor:          cfg.LLM.DefaultVendor,
		Model:           cfg.LLM.DefaultModel,
		OpenAIKey:       firstNonEmpty(os.Getenv("OPENAI_API_KEY"), cfg.APIKeys.OpenAI, cfg.LLM.OpenAIKey),
		AnthropicKey:    firstNonEmpty(os.Getenv("ANTHROPIC_API_KEY"), cfg.APIKeys.Anthropic, cfg.LLM.AnthropicKey),
		GeminiKey:       firstNonEmpty(os.Getenv("GEMINI_API_KEY"), cfg.APIKeys.Gemini, cfg.LLM.GeminiKey),
		OpenRouterKey:   firstNonEmpty(os.Getenv("OPENROUTER_API_KEY"), cfg.APIKeys.OpenRouter),
		OpenAIBaseURL:   cfg.LLM.OpenAIBaseURL,
		OpenRouterTitle: cfg.LLM.OpenRouterTitle,
		ReferrerURL:     cfg.LLM.ReferrerURL,
		LocalURL:        cfg.LLM.LocalURL,
		LocalKey:        firstNonEmpty(os.Getenv("LOCAL_LLM_API_KEY"), cfg.LLM.LocalKey),
	}
}

func EmbeddingSettings(cfg config.Config) embeddings.ProviderSettings {
	return embeddings.ProviderSettings{
		Provider:      cfg.LLM.EmbeddingProvider,
		Model:         cfg.LLM.EmbeddingModel,
		Dimensions:    cfg.LLM.EmbeddingDim,
		OpenAIKey:     firstNonEmpty(os.Getenv("OPENAI_API_KEY"), cfg.APIKeys.OpenAI, cfg.LLM.OpenAIKey),
		GeminiKey:     firstNonEmpty(os.Getenv("GEMINI_API_KEY"), cfg.APIKeys.Gemini, cfg.LLM.GeminiKey),
		VoyageKey:     firstNonEmpty(os.Getenv("VOYAGE_API_KEY"), cfg.APIKeys.Voyage),
		OpenRouterKey: firstNonEmpty(os.Getenv("OPENROUTER_API_KEY"), cfg.APIKeys.OpenRouter),
		LocalURL:      cfg.LLM.LocalURL,
	}
}

func Resolver(cfg config.Config, db tenant.Querier) (*llm.CompanyResolver, error) {
	var lookup llm.AgentLookup
	if db != nil && cfg.LLM.Enabled {
		lookup = mysqlai.NewAgentLookup(db, cfg.LLM.ProviderConfigKey)
	}
	resolver, err := llm.NewCompanyResolver(lookup, LLMSettings(cfg))
	if err != nil {
		return nil, err
	}
	resolver.
		WithResilient(resilientOptions(cfg)).
		WithCacheTTL(time.Duration(cfg.LLM.ResolverCacheTTLSeconds) * time.Second)
	return resolver, nil
}

func resilientOptions(cfg config.Config) llm.ResilientOptions {
	return llm.ResilientOptions{
		Timeout:          time.Duration(cfg.LLM.RequestTimeoutSeconds) * time.Second,
		RequestsPerSec:   cfg.LLM.RateLimitPerSec,
		Burst:            cfg.LLM.RateLimitBurst,
		BreakerFailures:  cfg.LLM.BreakerFailures,
		BreakerCooldown:  time.Duration(cfg.LLM.BreakerCooldownSeconds) * time.Second,
		MaxRetries:       cfg.LLM.MaxRetries,
		RetryBaseBackoff: time.Duration(cfg.LLM.RetryBaseBackoffMs) * time.Millisecond,
		RetryMaxBackoff:  time.Duration(cfg.LLM.RetryMaxBackoffMs) * time.Millisecond,
	}
}
