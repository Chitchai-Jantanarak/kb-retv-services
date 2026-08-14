package config

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

func LoadFrom(path string) (Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	for _, binding := range envBindings {
		_ = v.BindEnv(binding.key, binding.env)
	}

	setDefaults(v)

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			var notFound viper.ConfigFileNotFoundError
			if !errors.As(err, &notFound) {
				return Config{}, err
			}
		}
		if err := loadDotEnv(v, filepath.Join(filepath.Dir(path), ".env")); err != nil {
			return Config{}, err
		}
	}

	return configFrom(v), nil
}

func configFrom(v *viper.Viper) Config {
	return Config{
		App: App{
			Env: v.GetString("app.env"),
			Key: v.GetString("app.key"),
		},
		Server: Server{
			Port:               v.GetString("server.port"),
			RequestBudgetMs:    v.GetInt("server.request_budget_ms"),
			DeadlineHeadroomMs: v.GetInt("server.deadline_headroom_ms"),
			DeadlineMinMs:      v.GetInt("server.deadline_min_ms"),
			DeadlineMaxMs:      v.GetInt("server.deadline_max_ms"),
		},
		MySQL: MySQL{
			Enabled:      v.GetBool("mysql.enabled"),
			DSN:          v.GetString("mysql.dsn"),
			MaxOpenConns: v.GetInt("mysql.maxOpenConns"),
			MaxIdleConns: v.GetInt("mysql.maxIdleConns"),
		},
		Redis: Redis{
			URL: v.GetString("redis.url"),
		},
		Qdrant: Qdrant{
			Enabled:          v.GetBool("qdrant.enabled"),
			URL:              v.GetString("qdrant.url"),
			APIKey:           firstNonEmpty(v.GetString("qdrant.api_key"), v.GetString("qdrant.apiKey")),
			CollectionPrefix: v.GetString("qdrant.collectionPrefix"),
		},
		Memgraph: Memgraph{
			Enabled:  v.GetBool("memgraph.enabled"),
			URI:      v.GetString("memgraph.uri"),
			Username: v.GetString("memgraph.username"),
			Password: v.GetString("memgraph.password"),
		},
		LLM: LLM{
			Enabled:       v.GetBool("llm.enabled"),
			DefaultVendor: v.GetString("llm.default_vendor"),
			DefaultModel:  v.GetString("llm.default_model"),
			EmbeddingProvider: firstNonEmpty(
				v.GetString("llm.embedding_provider"),
				v.GetString("llm.default_vendor"),
			),
			EmbeddingModel:    v.GetString("llm.embedding_model"),
			EmbeddingDim:      v.GetInt("llm.embedding_dim"),
			OpenAIKey:         firstNonEmpty(v.GetString("apiKeys.openai"), v.GetString("llm.openai_key")),
			OpenAIBaseURL:     v.GetString("llm.openai_base_url"),
			GeminiKey:         firstNonEmpty(v.GetString("apiKeys.gemini"), v.GetString("llm.gemini_key")),
			AnthropicKey:      firstNonEmpty(v.GetString("apiKeys.anthropic"), v.GetString("llm.anthropic_key")),
			OpenRouterTitle:   v.GetString("llm.openrouter_title"),
			ReferrerURL:       v.GetString("llm.referrer_url"),
			LocalURL:          v.GetString("llm.local_url"),
			LocalKey:          v.GetString("llm.local_key"),
			ProviderConfigKey: v.GetString("llm.provider_config_key"),

			RequestTimeoutSeconds:   v.GetInt("llm.request_timeout_seconds"),
			RateLimitPerSec:         v.GetFloat64("llm.rate_limit_per_sec"),
			RateLimitBurst:          v.GetInt("llm.rate_limit_burst"),
			BreakerFailures:         v.GetInt("llm.breaker_failures"),
			BreakerCooldownSeconds:  v.GetInt("llm.breaker_cooldown_seconds"),
			MaxRetries:              v.GetInt("llm.max_retries"),
			RetryBaseBackoffMs:      v.GetInt("llm.retry_base_backoff_ms"),
			RetryMaxBackoffMs:       v.GetInt("llm.retry_max_backoff_ms"),
			ResolverCacheTTLSeconds: v.GetInt("llm.resolver_cache_ttl_seconds"),
		},
		APIKeys: APIKeys{
			OpenAI:     firstNonEmpty(v.GetString("apiKeys.openai"), v.GetString("llm.openai_key")),
			Gemini:     firstNonEmpty(v.GetString("apiKeys.gemini"), v.GetString("llm.gemini_key")),
			Anthropic:  firstNonEmpty(v.GetString("apiKeys.anthropic"), v.GetString("llm.anthropic_key")),
			Voyage:     v.GetString("apiKeys.voyage"),
			OpenRouter: v.GetString("apiKeys.openrouter"),
		},
		Logger: Logger{
			Level:      v.GetString("logger.level"),
			Format:     v.GetString("logger.format"),
			OutputPath: v.GetString("logger.outputPath"),
		},
		Swagger: Swagger{
			Enabled: v.GetBool("swagger.enabled"),
		},
		Laravel: Laravel{
			BaseURL:          v.GetString("laravel.base_url"),
			HostHeader:       v.GetString("laravel.host_header"),
			WebhookSecret:    v.GetString("laravel.webhook_secret"),
			TicketPath:       firstNonEmpty(v.GetString("laravel.ticket_path"), "/api/webhooks/ai/ticket-create"),
			Timeout:          firstNonZero(v.GetInt("laravel.timeout"), 10),
			JWTSecret:        v.GetString("laravel.jwt_secret"),
			JWTPublicKeyPath: v.GetString("laravel.jwt_public_key_path"),
			JWKSURL:          v.GetString("laravel.jwks_url"),
		},
		Line: Line{
			ChannelSecret:      v.GetString("line.channel_secret"),
			ChannelAccessToken: v.GetString("line.channel_access_token"),
		},
		Budget: Budget{
			MaxLLMCallsPerRequest:       v.GetInt("budget.maxLLMCallsPerRequest"),
			MaxLLMCallsPerCompanyWindow: v.GetInt("budget.maxLLMCallsPerCompanyWindow"),
			WindowSeconds:               v.GetInt("budget.windowSeconds"),
		},
		Chat: Chat{
			CacheTTLSeconds:       v.GetInt("chat.cacheTTLSeconds"),
			CacheMaxEntries:       v.GetInt("chat.cacheMaxEntries"),
			GuardEmbedderProvider: v.GetString("chat.guardEmbedderProvider"),
			GuardEmbedderAssetDir: v.GetString("chat.guardEmbedderAssetDir"),
			SelectorRejectMargin:  v.GetFloat64("chat.selectorRejectMargin"),
		},
		Embedding: Embedding{
			RefreshMaxChunks:  v.GetInt("embedding.refreshMaxChunks"),
			RefreshBatchSize:  v.GetInt("embedding.refreshBatchSize"),
			LargeRunThreshold: v.GetInt("embedding.largeRunThreshold"),
		},
		Reply: Reply{
			DefaultMode: v.GetString("reply.defaultMode"),
		},
	}
}
