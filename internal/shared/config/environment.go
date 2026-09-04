package config

import (
	"bufio"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type envBinding struct {
	key string
	env string
}

var envBindings = []envBinding{
	{key: "app.env", env: "APP_ENV"},
	{key: "app.dev_auth_bypass", env: "APP_DEV_AUTH_BYPASS"},
	{key: "server.port", env: "SERVER_PORT"},
	{key: "server.request_budget_ms", env: "SERVER_REQUEST_BUDGET_MS"},
	{key: "server.deadline_headroom_ms", env: "SERVER_DEADLINE_HEADROOM_MS"},
	{key: "server.deadline_min_ms", env: "SERVER_DEADLINE_MIN_MS"},
	{key: "server.deadline_max_ms", env: "SERVER_DEADLINE_MAX_MS"},
	{key: "budget.maxLLMCallsPerRequest", env: "BUDGET_MAX_LLM_CALLS_PER_REQUEST"},
	{key: "budget.maxLLMCallsPerCompanyWindow", env: "BUDGET_MAX_LLM_CALLS_PER_COMPANY_WINDOW"},
	{key: "budget.windowSeconds", env: "BUDGET_WINDOW_SECONDS"},
	{key: "chat.cacheTTLSeconds", env: "CHAT_CACHE_TTL_SECONDS"},
	{key: "chat.cacheMaxEntries", env: "CHAT_CACHE_MAX_ENTRIES"},
	{key: "chat.guardEmbedderProvider", env: "CHAT_GUARDEMBEDDERPROVIDER"},
	{key: "chat.guardEmbedderAssetDir", env: "CHAT_GUARDEMBEDDERASSETDIR"},
	{key: "chat.selectorRejectMargin", env: "CHAT_SELECTORREJECTMARGIN"},
	{key: "chat.offTopicMargin", env: "CHAT_OFFTOPICMARGIN"},
	{key: "embedding.refreshMaxChunks", env: "EMBEDDING_REFRESH_MAXCHUNKS"},
	{key: "embedding.refreshBatchSize", env: "EMBEDDING_REFRESH_BATCHSIZE"},
	{key: "embedding.largeRunThreshold", env: "EMBEDDING_LARGE_RUN_THRESHOLD"},
	{key: "reply.defaultMode", env: "REPLY_DEFAULT_MODE"},
	{key: "intake.assess_mode", env: "INTAKE_ASSESS_MODE"},
	{key: "intake.assess_timeout_ms", env: "INTAKE_ASSESS_TIMEOUT_MS"},
	{key: "entitlements.cache_ttl_seconds", env: "ENTITLEMENTS_CACHE_TTL_SECONDS"},
	{key: "mysql.enabled", env: "MYSQL_ENABLED"},
	{key: "mysql.dsn", env: "MYSQL_DSN"},
	{key: "mysql.maxOpenConns", env: "MYSQL_MAXOPENCONNS"},
	{key: "mysql.maxIdleConns", env: "MYSQL_MAXIDLECONNS"},
	{key: "qdrant.enabled", env: "QDRANT_ENABLED"},
	{key: "qdrant.url", env: "QDRANT_URL"},
	{key: "qdrant.apiKey", env: "QDRANT_APIKEY"},
	{key: "qdrant.collectionPrefix", env: "QDRANT_COLLECTIONPREFIX"},
	{key: "memgraph.enabled", env: "MEMGRAPH_ENABLED"},
	{key: "memgraph.uri", env: "MEMGRAPH_URI"},
	{key: "memgraph.username", env: "MEMGRAPH_USERNAME"},
	{key: "memgraph.password", env: "MEMGRAPH_PASSWORD"},
	{key: "redis.url", env: "REDIS_URL"},
	{key: "logger.level", env: "LOGGER_LEVEL"},
	{key: "logger.format", env: "LOGGER_FORMAT"},
	{key: "logger.outputPath", env: "LOGGER_OUTPUTPATH"},
	{key: "swagger.enabled", env: "SWAGGER_ENABLED"},
	{key: "apiKeys.openai", env: "APIKEYS_OPENAI"},
	{key: "apiKeys.gemini", env: "APIKEYS_GEMINI"},
	{key: "apiKeys.anthropic", env: "APIKEYS_ANTHROPIC"},
	{key: "apiKeys.voyage", env: "APIKEYS_VOYAGE"},
	{key: "apiKeys.openrouter", env: "APIKEYS_OPENROUTER"},
	{key: "llm.enabled", env: "LLM_ENABLED"},
	{key: "llm.default_vendor", env: "LLM_DEFAULT_VENDOR"},
	{key: "llm.default_model", env: "LLM_DEFAULT_MODEL"},
	{key: "llm.embedding_provider", env: "LLM_EMBEDDING_PROVIDER"},
	{key: "llm.embedding_model", env: "LLM_EMBEDDING_MODEL"},
	{key: "llm.embedding_dim", env: "LLM_EMBEDDING_DIM"},
	{key: "llm.openai_base_url", env: "LLM_OPENAI_BASE_URL"},
	{key: "llm.openrouter_title", env: "LLM_OPENROUTER_TITLE"},
	{key: "llm.referrer_url", env: "LLM_REFERRER_URL"},
	{key: "llm.local_url", env: "LLM_LOCAL_URL"},
	{key: "llm.local_key", env: "LOCAL_LLM_API_KEY"},
	{key: "llm.provider_config_key", env: "AI_PROVIDER_KEY"},
	{key: "llm.request_timeout_seconds", env: "LLM_REQUEST_TIMEOUT_SECONDS"},
	{key: "llm.max_retries", env: "LLM_MAX_RETRIES"},
	{key: "llm.retry_base_backoff_ms", env: "LLM_RETRY_BASE_BACKOFF_MS"},
	{key: "llm.retry_max_backoff_ms", env: "LLM_RETRY_MAX_BACKOFF_MS"},
	{key: "llm.rate_limit_per_sec", env: "LLM_RATE_LIMIT_PER_SEC"},
	{key: "llm.rate_limit_burst", env: "LLM_RATE_LIMIT_BURST"},
	{key: "llm.breaker_failures", env: "LLM_BREAKER_FAILURES"},
	{key: "llm.breaker_cooldown_seconds", env: "LLM_BREAKER_COOLDOWN_SECONDS"},
	{key: "llm.resolver_cache_ttl_seconds", env: "LLM_RESOLVER_CACHE_TTL_SECONDS"},
	{key: "laravel.base_url", env: "LARAVEL_BASE_URL"},
	{key: "laravel.host_header", env: "LARAVEL_HOST_HEADER"},
	{key: "laravel.webhook_secret", env: "LARAVEL_WEBHOOK_SECRET"},
	{key: "laravel.ticket_path", env: "LARAVEL_TICKET_PATH"},
	{key: "laravel.timeout", env: "LARAVEL_TIMEOUT"},
	{key: "laravel.jwt_secret", env: "AI_SERVICE_JWT_SECRET"},
	{key: "laravel.jwt_public_key_path", env: "JWT_PUBLIC_KEY_PATH"},
	{key: "laravel.jwks_url", env: "JWT_JWKS_URL"},
	{key: "line.channel_secret", env: "LINE_CHANNEL_SECRET"},
	{key: "line.channel_access_token", env: "LINE_CHANNEL_ACCESS_TOKEN"},
}

func loadDotEnv(v *viper.Viper, path string) error {
	file, err := os.Open(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		name, value, ok := parseEnvLine(scanner.Text())
		if ok {
			values[name] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	for _, binding := range envBindings {
		value, ok := values[binding.env]
		if !ok {
			continue
		}
		if _, ok := os.LookupEnv(binding.env); ok {
			continue
		}
		v.Set(binding.key, value)
	}
	return nil
}

func parseEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}

	name, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", false
	}

	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return name, value, true
}
