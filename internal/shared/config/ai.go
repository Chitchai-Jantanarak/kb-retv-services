package config

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
