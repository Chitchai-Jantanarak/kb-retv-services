package main

import (
	"context"
	"fmt"
	"log"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/application/services/vectorindex"
	"github.com/my/app/internal/domain/ports"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	"github.com/my/app/internal/infra/qdrant"
	mysqlkb "github.com/my/app/internal/repositories/kb/mysql"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

func buildRefreshHandler(cfg config.Config) taskHandler {
	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil || db == nil {
		log.Printf("embedding:refresh disabled: mysql unavailable")
		return unavailable("embedding:refresh", "mysql not configured")
	}
	qdb, err := buildTenantQuerier(cfg, db)
	if err != nil {
		log.Printf("embedding:refresh disabled: tenant router unavailable: %v", err)
		return unavailable("embedding:refresh", "tenant router not configured")
	}
	provider, modelName, embedder, err := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg))
	if err != nil {
		log.Printf("embedding:refresh disabled: build embedder: %v", err)
		return unavailable("embedding:refresh", "embedder unavailable")
	}
	collectionPrefix := cfg.Qdrant.CollectionPrefix
	if collectionPrefix == "" {
		collectionPrefix = "kb_chunks"
	}
	indexer := vectorindex.NewIndexer(
		mysqlkb.NewChunkSourceForModel(qdb, modelName),
		embedder,
		qdrant.NewStore(qdrant.Config{URL: cfg.Qdrant.URL, APIKey: cfg.Qdrant.APIKey}),
		mysqlkb.NewEmbeddingMarker(qdb),
	)
	log.Printf("embedding:refresh handler configured: provider=%s model=%s", provider, modelName)

	return func(ctx context.Context, job ports.Job) error {
		result, err := indexer.Run(ctx, vectorindex.Options{
			CompanyID:        job.CompanyID,
			CollectionPrefix: collectionPrefix,
			BatchSize:        cfg.Embedding.RefreshBatchSize,
			Limit:            cfg.Embedding.RefreshMaxChunks,
			Model:            modelName,
		})
		if err != nil {
			return fmt.Errorf("embedding:refresh: %w", err)
		}
		log.Printf("embedding:refresh result company=%d chunks=%d points=%d batches=%d",
			job.CompanyID, result.Chunks, result.Points, result.Batches)
		return nil
	}
}
