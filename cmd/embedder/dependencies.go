package main

import (
	"database/sql"
	"os"

	"github.com/my/app/internal/ai/embeddings"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	"github.com/my/app/internal/infra/tenant"
	"github.com/my/app/internal/shared/config"
)

func buildTenantQuerier(cfg config.Config, db *sql.DB) (tenant.Querier, func(), error) {
	pool, err := tenant.NewPool(db, func(name string) (*sql.DB, error) {
		return infra_mysql.OpenForDB(cfg.MySQL, name)
	})
	if err != nil {
		return nil, nil, err
	}
	return pool.Router(), pool.Close, nil
}

func embeddingSettings(opts commandOptions, cfg config.Config) embeddings.ProviderSettings {
	provider := opts.provider
	if provider == "auto" && cfg.LLM.EmbeddingProvider != "" {
		provider = cfg.LLM.EmbeddingProvider
	}
	model := opts.model
	if model == "" {
		model = cfg.LLM.EmbeddingModel
	}
	dim := opts.dim
	if dim <= 0 {
		dim = cfg.LLM.EmbeddingDim
	}
	return embeddings.ProviderSettings{
		Provider:       provider,
		Model:          model,
		BaseURL:        opts.baseURL,
		Dimensions:     dim,
		MaxRetries:     opts.embedRetries,
		RequestsPerMin: opts.embedRPM,
		OpenAIKey:      firstNonEmpty(os.Getenv("OPENAI_API_KEY"), cfg.APIKeys.OpenAI, cfg.LLM.OpenAIKey),
		GeminiKey:      firstNonEmpty(os.Getenv("GEMINI_API_KEY"), cfg.APIKeys.Gemini, cfg.LLM.GeminiKey),
		VoyageKey:      firstNonEmpty(os.Getenv("VOYAGE_API_KEY"), cfg.APIKeys.Voyage),
		OpenRouterKey:  firstNonEmpty(os.Getenv("OPENROUTER_API_KEY"), cfg.APIKeys.OpenRouter),
		LocalURL:       cfg.LLM.LocalURL,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
