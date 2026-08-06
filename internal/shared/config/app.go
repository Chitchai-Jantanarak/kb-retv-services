package config

import "strings"

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

type Budget struct {
	MaxLLMCallsPerRequest       int
	MaxLLMCallsPerCompanyWindow int
	WindowSeconds               int
}

type Chat struct {
	CacheTTLSeconds       int
	CacheMaxEntries       int
	GuardEmbedderProvider string
	GuardEmbedderAssetDir string
}

type Embedding struct {
	RefreshMaxChunks  int
	RefreshBatchSize  int
	LargeRunThreshold int
}

type Reply struct {
	DefaultMode string
}
