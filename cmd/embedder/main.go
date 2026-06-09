package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/application/services/vectorindex"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	"github.com/my/app/internal/infra/qdrant"
	"github.com/my/app/internal/infra/tenant"
	mysqlkb "github.com/my/app/internal/repositories/kb/mysql"
	"github.com/my/app/internal/shared/config"
)

func main() {
	task := flag.String("task", "index-kb", "embedder task")
	company := flag.Int64("company", 0, "company id to scope indexing (0 = all)")
	collection := flag.String("collection", "", "qdrant collection")
	batchSize := flag.Int("batch-size", 64, "chunks per embedding batch")
	limit := flag.Int("limit", 0, "maximum chunks to index")
	provider := flag.String("provider", "auto", "embedding provider: auto, openai, openrouter, gemini, voyage, claude, ollama, or hash")
	dim := flag.Int("dim", 0, "embedding dimensions")
	model := flag.String("model", "", "embedding model name")
	baseURL := flag.String("base-url", "", "OpenAI-compatible base URL")
	dryRun := flag.Bool("dry-run", false, "count chunks without embedding")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if *collection == "" {
		*collection = cfg.Qdrant.CollectionPrefix
	}

	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	if db == nil {
		log.Fatal("mysql is disabled")
	}
	defer db.Close()
	qdb, closeRouter, err := buildTenantQuerier(cfg, db)
	if err != nil {
		log.Fatalf("build tenant router: %v", err)
	}
	defer closeRouter()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	switch *task {
	case "index-kb":
		resolvedProvider, modelName, embedder, err := embeddings.NewProvider(embeddingSettings(*provider, *model, *dim, *baseURL, cfg))
		if err != nil {
			log.Fatalf("build embedder: %v", err)
		}
		indexer := vectorindex.NewIndexer(
			mysqlkb.NewChunkSourceForModel(qdb, modelName),
			embedder,
			qdrant.NewStore(qdrant.Config{URL: cfg.Qdrant.URL, APIKey: cfg.Qdrant.APIKey}),
			mysqlkb.NewEmbeddingMarker(qdb),
		)
		if *company == 0 {
			log.Println("warning: -company=0 scans chunks across every company; pass -company=<id> to scope")
		}
		result, err := indexer.Run(ctx, vectorindex.Options{
			CompanyID:        *company,
			CollectionPrefix: *collection,
			BatchSize:        *batchSize,
			Limit:            *limit,
			Model:            modelName,
			DryRun:           *dryRun,
		})
		if err != nil {
			log.Fatalf("index kb: %v", err)
		}
		fmt.Printf("kb index complete: chunks=%d points=%d batches=%d collection=%s model=%s provider=%s dim=%d\n", result.Chunks, result.Points, result.Batches, *collection, modelName, resolvedProvider, embedder.Dim())
	default:
		log.Fatalf("unknown task %q", *task)
	}
}

func buildTenantQuerier(cfg config.Config, db *sql.DB) (tenant.Querier, func(), error) {
	pool, err := tenant.NewPool(db, func(name string) (*sql.DB, error) {
		return infra_mysql.OpenForDB(cfg.MySQL, name)
	})
	if err != nil {
		return nil, nil, err
	}
	return pool.Router(), pool.Close, nil
}

func embeddingSettings(provider string, model string, dim int, baseURL string, cfg config.Config) embeddings.ProviderSettings {
	if provider == "auto" && cfg.LLM.EmbeddingProvider != "" {
		provider = cfg.LLM.EmbeddingProvider
	}
	if model == "" {
		model = cfg.LLM.EmbeddingModel
	}
	if dim <= 0 {
		dim = cfg.LLM.EmbeddingDim
	}
	return embeddings.ProviderSettings{
		Provider:      provider,
		Model:         model,
		BaseURL:       baseURL,
		Dimensions:    dim,
		OpenAIKey:     firstNonEmpty(os.Getenv("OPENAI_API_KEY"), cfg.APIKeys.OpenAI, cfg.LLM.OpenAIKey),
		GeminiKey:     firstNonEmpty(os.Getenv("GEMINI_API_KEY"), cfg.APIKeys.Gemini, cfg.LLM.GeminiKey),
		VoyageKey:     firstNonEmpty(os.Getenv("VOYAGE_API_KEY"), cfg.APIKeys.Voyage),
		OpenRouterKey: firstNonEmpty(os.Getenv("OPENROUTER_API_KEY"), cfg.APIKeys.OpenRouter),
		LocalURL:      cfg.LLM.LocalURL,
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
