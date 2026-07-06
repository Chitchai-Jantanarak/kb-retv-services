package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromReadsConfigFileAndDerivesProductionSwagger(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte(`
app:
  env: production
server:
  port: "9000"
swagger:
  enabled: true
mysql:
  enabled: true
  dsn: "user:pass@tcp(localhost:3306)/db"
  maxOpenConns: 11
  maxIdleConns: 3
qdrant:
  enabled: true
  url: "http://qdrant:6333"
  collectionPrefix: "rag"
memgraph:
  enabled: true
  uri: "bolt://memgraph:7687"
  username: "memgraph"
  password: "secret"
apiKeys:
  openai: "openai-test"
  gemini: "gemini-test"
  anthropic: "anthropic-test"
  voyage: "voyage-test"
  openrouter: "openrouter-test"
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	if cfg.Server.Port != "9000" {
		t.Fatalf("Server.Port = %q, want 9000", cfg.Server.Port)
	}
	if !cfg.MySQL.Enabled {
		t.Fatal("MySQL.Enabled = false, want true")
	}
	if cfg.MySQL.MaxOpenConns != 11 {
		t.Fatalf("MySQL.MaxOpenConns = %d, want 11", cfg.MySQL.MaxOpenConns)
	}
	if cfg.Swagger.EnabledFor(cfg.App) {
		t.Fatal("Swagger.EnabledFor(production) = true, want false")
	}
	if !cfg.Qdrant.Enabled {
		t.Fatal("Qdrant.Enabled = false, want true")
	}
	if cfg.Qdrant.CollectionPrefix != "rag" {
		t.Fatalf("Qdrant.CollectionPrefix = %q, want rag", cfg.Qdrant.CollectionPrefix)
	}
	if !cfg.Memgraph.Enabled {
		t.Fatal("Memgraph.Enabled = false, want true")
	}
	if cfg.Memgraph.URI != "bolt://memgraph:7687" {
		t.Fatalf("Memgraph.URI = %q, want bolt://memgraph:7687", cfg.Memgraph.URI)
	}
	if cfg.APIKeys.OpenAI != "openai-test" {
		t.Fatalf("APIKeys.OpenAI = %q, want openai-test", cfg.APIKeys.OpenAI)
	}
	if cfg.LLM.OpenAIKey != "openai-test" {
		t.Fatalf("LLM.OpenAIKey = %q, want openai-test", cfg.LLM.OpenAIKey)
	}
	if cfg.LLM.EmbeddingProvider != "gemini" {
		t.Fatalf("LLM.EmbeddingProvider = %q, want gemini", cfg.LLM.EmbeddingProvider)
	}
	if cfg.APIKeys.Gemini != "gemini-test" {
		t.Fatalf("APIKeys.Gemini = %q, want gemini-test", cfg.APIKeys.Gemini)
	}
	if cfg.APIKeys.Anthropic != "anthropic-test" {
		t.Fatalf("APIKeys.Anthropic = %q, want anthropic-test", cfg.APIKeys.Anthropic)
	}
	if cfg.APIKeys.Voyage != "voyage-test" {
		t.Fatalf("APIKeys.Voyage = %q, want voyage-test", cfg.APIKeys.Voyage)
	}
	if cfg.APIKeys.OpenRouter != "openrouter-test" {
		t.Fatalf("APIKeys.OpenRouter = %q, want openrouter-test", cfg.APIKeys.OpenRouter)
	}
}

func TestLoadDefaultsToDevSwaggerEnabled(t *testing.T) {
	cfg, err := LoadFrom("")
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	if cfg.App.Env != "development" {
		t.Fatalf("App.Env = %q, want development", cfg.App.Env)
	}
	if !cfg.Swagger.EnabledFor(cfg.App) {
		t.Fatal("Swagger.EnabledFor(development) = false, want true")
	}
	if cfg.Qdrant.CollectionPrefix != "kb_chunks" {
		t.Fatalf("Qdrant.CollectionPrefix = %q, want kb_chunks", cfg.Qdrant.CollectionPrefix)
	}
	if cfg.Memgraph.URI != "bolt://localhost:7687" {
		t.Fatalf("Memgraph.URI = %q, want bolt://localhost:7687", cfg.Memgraph.URI)
	}
}

func TestLoadFromReadsDotEnvNextToConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	envPath := filepath.Join(dir, ".env")

	if err := os.WriteFile(configPath, []byte("app:\n  env: development\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	if err := os.WriteFile(envPath, []byte(`
APP_ENV=production
SERVER_PORT=7070
MYSQL_ENABLED=true
MYSQL_DSN=user:pass@tcp(localhost:33061)/tenancy
QDRANT_URL=http://qdrant:6333
APIKEYS_OPENAI=openai-env
APIKEYS_GEMINI=gemini-env
APIKEYS_VOYAGE=voyage-env
APIKEYS_OPENROUTER=openrouter-env
LLM_EMBEDDING_PROVIDER=openrouter
LLM_EMBEDDING_MODEL=nvidia/llama-nemotron-embed-vl-1b-v2:free
LLM_EMBEDDING_DIM=2048
`), 0o600); err != nil {
		t.Fatalf("WriteFile(env) error = %v", err)
	}

	cfg, err := LoadFrom(configPath)
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	if cfg.App.Env != "production" {
		t.Fatalf("App.Env = %q, want production", cfg.App.Env)
	}
	if cfg.Server.Port != "7070" {
		t.Fatalf("Server.Port = %q, want 7070", cfg.Server.Port)
	}
	if !cfg.MySQL.Enabled {
		t.Fatal("MySQL.Enabled = false, want true")
	}
	if cfg.MySQL.DSN != "user:pass@tcp(localhost:33061)/tenancy" {
		t.Fatalf("MySQL.DSN = %q, want env DSN", cfg.MySQL.DSN)
	}
	if cfg.Qdrant.URL != "http://qdrant:6333" {
		t.Fatalf("Qdrant.URL = %q, want http://qdrant:6333", cfg.Qdrant.URL)
	}
	if cfg.APIKeys.OpenAI != "openai-env" {
		t.Fatalf("APIKeys.OpenAI = %q, want openai-env", cfg.APIKeys.OpenAI)
	}
	if cfg.APIKeys.Gemini != "gemini-env" {
		t.Fatalf("APIKeys.Gemini = %q, want gemini-env", cfg.APIKeys.Gemini)
	}
	if cfg.APIKeys.Voyage != "voyage-env" {
		t.Fatalf("APIKeys.Voyage = %q, want voyage-env", cfg.APIKeys.Voyage)
	}
	if cfg.APIKeys.OpenRouter != "openrouter-env" {
		t.Fatalf("APIKeys.OpenRouter = %q, want openrouter-env", cfg.APIKeys.OpenRouter)
	}
	if cfg.LLM.EmbeddingProvider != "openrouter" {
		t.Fatalf("LLM.EmbeddingProvider = %q, want openrouter", cfg.LLM.EmbeddingProvider)
	}
	if cfg.LLM.EmbeddingModel != "nvidia/llama-nemotron-embed-vl-1b-v2:free" {
		t.Fatalf("LLM.EmbeddingModel = %q, want nvidia free model", cfg.LLM.EmbeddingModel)
	}
	if cfg.LLM.EmbeddingDim != 2048 {
		t.Fatalf("LLM.EmbeddingDim = %d, want 2048", cfg.LLM.EmbeddingDim)
	}
}

func TestChatCacheDefaults(t *testing.T) {
	cfg, err := LoadFrom("")
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	if cfg.Chat.CacheTTLSeconds != 300 {
		t.Fatalf("Chat.CacheTTLSeconds = %d, want 300", cfg.Chat.CacheTTLSeconds)
	}
	if cfg.Chat.CacheMaxEntries != 4096 {
		t.Fatalf("Chat.CacheMaxEntries = %d, want 4096", cfg.Chat.CacheMaxEntries)
	}
}
