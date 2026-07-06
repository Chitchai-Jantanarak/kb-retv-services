package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	App       App
	Server    Server
	MySQL     MySQL
	Redis     Redis
	Qdrant    Qdrant
	Memgraph  Memgraph
	LLM       LLM
	APIKeys   APIKeys
	Logger    Logger
	Swagger   Swagger
	Laravel   Laravel
	Budget    Budget
	Chat      Chat
	Embedding Embedding
}

type Budget struct {
	MaxLLMCallsPerRequest       int
	MaxLLMCallsPerCompanyWindow int
	WindowSeconds               int
}

type Chat struct {
	CacheTTLSeconds int
	CacheMaxEntries int
}

type Embedding struct {
	RefreshMaxChunks  int
	RefreshBatchSize  int
	LargeRunThreshold int
}

type Laravel struct {
	BaseURL          string
	WebhookSecret    string
	TicketPath       string
	Timeout          int
	JWTSecret        string
	JWTPublicKeyPath string
	JWKSURL          string
}

type App struct {
	Env string
}

func (a App) IsProduction() bool {
	env := strings.ToLower(strings.TrimSpace(a.Env))
	return env == "prod" || env == "production"
}

type Server struct {
	Port               string
	RequestBudgetMs    int
	DeadlineHeadroomMs int
	DeadlineMinMs      int
	DeadlineMaxMs      int
}

type MySQL struct {
	Enabled      bool
	DSN          string
	MaxOpenConns int
	MaxIdleConns int
}

type Redis struct {
	URL string
}

type Qdrant struct {
	Enabled          bool
	URL              string
	APIKey           string
	CollectionPrefix string
}

type Memgraph struct {
	Enabled  bool
	URI      string
	URL      string
	Username string
	Password string
}

type LLM struct {
	DefaultVendor     string
	DefaultModel      string
	EmbeddingProvider string
	EmbeddingModel    string
	EmbeddingDim      int
	OpenAIKey         string
	OpenAIBaseURL     string
	GeminiKey         string
	AnthropicKey      string
	OpenRouterTitle   string
	ReferrerURL       string
	LocalURL          string
	LocalKey          string
	ProviderConfigKey string

	RequestTimeoutSeconds   int
	RateLimitPerSec         float64
	RateLimitBurst          int
	BreakerFailures         int
	BreakerCooldownSeconds  int
	MaxRetries              int
	RetryBaseBackoffMs      int
	RetryMaxBackoffMs       int
	ResolverCacheTTLSeconds int
}

type APIKeys struct {
	OpenAI     string
	Gemini     string
	Anthropic  string
	Voyage     string
	OpenRouter string
}

type Logger struct {
	Level      string
	Format     string
	OutputPath string
}

type Swagger struct {
	Enabled bool
}

func (s Swagger) EnabledFor(app App) bool {
	return s.Enabled && !app.IsProduction()
}

func Load() (Config, error) {
	return LoadFrom("config.yaml")
}

func LoadFrom(path string) (Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	v.SetDefault("app.env", "development")
	v.SetDefault("server.port", "8080")
	v.SetDefault("server.request_budget_ms", 22000)
	v.SetDefault("server.deadline_headroom_ms", 2000)
	v.SetDefault("server.deadline_min_ms", 1000)
	v.SetDefault("server.deadline_max_ms", 60000)
	v.SetDefault("mysql.enabled", false)
	v.SetDefault("mysql.maxOpenConns", 25)
	v.SetDefault("mysql.maxIdleConns", 5)
	v.SetDefault("qdrant.enabled", false)
	v.SetDefault("qdrant.url", "http://localhost:6333")
	v.SetDefault("qdrant.collectionPrefix", "kb_chunks")
	v.SetDefault("memgraph.enabled", false)
	v.SetDefault("memgraph.uri", "bolt://localhost:7687")
	v.SetDefault("llm.default_vendor", "gemini")
	v.SetDefault("llm.default_model", "gemini-2.5-flash")
	v.SetDefault("llm.request_timeout_seconds", 8)
	v.SetDefault("llm.breaker_failures", 5)
	v.SetDefault("llm.breaker_cooldown_seconds", 30)
	v.SetDefault("llm.max_retries", 1)
	v.SetDefault("llm.retry_base_backoff_ms", 400)
	v.SetDefault("llm.retry_max_backoff_ms", 1500)
	v.SetDefault("llm.rate_limit_per_sec", 0)
	v.SetDefault("llm.rate_limit_burst", 0)
	v.SetDefault("llm.resolver_cache_ttl_seconds", 60)
	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.format", "console")
	v.SetDefault("swagger.enabled", true)
	v.SetDefault("budget.maxLLMCallsPerRequest", 6)
	v.SetDefault("budget.maxLLMCallsPerCompanyWindow", 0)
	v.SetDefault("budget.windowSeconds", 60)
	v.SetDefault("chat.cacheTTLSeconds", 300)
	v.SetDefault("chat.cacheMaxEntries", 4096)
	v.SetDefault("embedding.refreshMaxChunks", 512)
	v.SetDefault("embedding.refreshBatchSize", 64)
	v.SetDefault("embedding.largeRunThreshold", 1000)

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

	cfg := Config{
		App: App{
			Env: v.GetString("app.env"),
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
			URL:      firstNonEmpty(v.GetString("memgraph.url"), v.GetString("memgraph.uri")),
			Username: v.GetString("memgraph.username"),
			Password: v.GetString("memgraph.password"),
		},
		LLM: LLM{
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
			WebhookSecret:    v.GetString("laravel.webhook_secret"),
			TicketPath:       firstNonEmpty(v.GetString("laravel.ticket_path"), "/api/webhooks/ai/ticket-create"),
			Timeout:          firstNonZero(v.GetInt("laravel.timeout"), 10),
			JWTSecret:        v.GetString("laravel.jwt_secret"),
			JWTPublicKeyPath: v.GetString("laravel.jwt_public_key_path"),
			JWKSURL:          v.GetString("laravel.jwks_url"),
		},
		Budget: Budget{
			MaxLLMCallsPerRequest:       v.GetInt("budget.maxLLMCallsPerRequest"),
			MaxLLMCallsPerCompanyWindow: v.GetInt("budget.maxLLMCallsPerCompanyWindow"),
			WindowSeconds:               v.GetInt("budget.windowSeconds"),
		},
		Chat: Chat{
			CacheTTLSeconds: v.GetInt("chat.cacheTTLSeconds"),
			CacheMaxEntries: v.GetInt("chat.cacheMaxEntries"),
		},
		Embedding: Embedding{
			RefreshMaxChunks:  v.GetInt("embedding.refreshMaxChunks"),
			RefreshBatchSize:  v.GetInt("embedding.refreshBatchSize"),
			LargeRunThreshold: v.GetInt("embedding.largeRunThreshold"),
		},
	}
	return cfg, nil
}

type envBinding struct {
	key string
	env string
}

var envBindings = []envBinding{
	{key: "app.env", env: "APP_ENV"},
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
	{key: "embedding.refreshMaxChunks", env: "EMBEDDING_REFRESH_MAXCHUNKS"},
	{key: "embedding.refreshBatchSize", env: "EMBEDDING_REFRESH_BATCHSIZE"},
	{key: "embedding.largeRunThreshold", env: "EMBEDDING_LARGE_RUN_THRESHOLD"},
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
	{key: "laravel.webhook_secret", env: "LARAVEL_WEBHOOK_SECRET"},
	{key: "laravel.ticket_path", env: "LARAVEL_TICKET_PATH"},
	{key: "laravel.timeout", env: "LARAVEL_TIMEOUT"},
	{key: "laravel.jwt_secret", env: "AI_SERVICE_JWT_SECRET"},
	{key: "laravel.jwt_public_key_path", env: "JWT_PUBLIC_KEY_PATH"},
	{key: "laravel.jwks_url", env: "JWT_JWKS_URL"},
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonZero(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}
