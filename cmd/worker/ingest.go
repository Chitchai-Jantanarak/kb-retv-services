package main

import (
	"context"
	"log"
	"time"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/application/services/kbbootstrap"
	"github.com/my/app/internal/application/services/vectorindex"
	"github.com/my/app/internal/application/workflows/ingest"
	"github.com/my/app/internal/domain/ports"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	"github.com/my/app/internal/infra/qdrant"
	mysqlkb "github.com/my/app/internal/repositories/kb/mysql"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

func buildIngestHandler(cfg config.Config) taskHandler {
	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil {
		log.Printf("ingest disabled: open mysql: %v", err)
		return unavailable("ingest", "mysql open failed")
	}
	if db == nil {
		log.Println("ingest disabled: mysql is not configured")
		return unavailable("ingest", "mysql not configured")
	}

	qdb, err := buildTenantQuerier(cfg, db)
	if err != nil {
		log.Printf("ingest disabled: tenant router unavailable: %v", err)
		return unavailable("ingest", "tenant router not configured")
	}

	provider, modelName, embedder, err := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg))
	if err != nil {
		log.Printf("ingest disabled: build embedder: %v", err)
		return unavailable("ingest", "embedder unavailable")
	}

	collectionPrefix := cfg.Qdrant.CollectionPrefix
	if collectionPrefix == "" {
		collectionPrefix = "kb_chunks"
	}

	workflow, err := ingest.New(ingest.Config{
		Bootstrapper: kbbootstrap.NewBootstrapper(
			mysqlkb.NewReportSource(qdb),
			mysqlkb.NewArticleStore(qdb),
		),
		Indexer: vectorindex.NewIndexer(
			mysqlkb.NewChunkSourceForModel(qdb, modelName),
			embedder,
			qdrant.NewStore(qdrant.Config{URL: cfg.Qdrant.URL, APIKey: cfg.Qdrant.APIKey}),
			mysqlkb.NewEmbeddingMarker(qdb),
		),
		CollectionPrefix: collectionPrefix,
		Model:            modelName,
		ChunkRunes:       kbbootstrap.DefaultChunkRunes,
	})
	if err != nil {
		log.Printf("ingest disabled: build workflow: %v", err)
		return unavailable("ingest", "workflow build failed")
	}

	log.Printf("ingest handler configured: provider=%s model=%s collection_prefix=%s",
		provider, modelName, collectionPrefix)

	graphWorkflow := buildGraphsyncWorkflow(cfg, qdb)
	if graphWorkflow != nil {
		log.Println("graphsync configured: ingest:report will sync to memgraph after successful index")
	}

	return func(ctx context.Context, job ports.Job) error {
		payload, err := decodeIngestPayload(job.Payload)
		if err != nil {
			return err
		}
		since := time.Now().Add(-5 * time.Second)
		result, err := workflow.Run(ctx, job.CompanyID, ingest.Options{
			Limit:     payload.Limit,
			BatchSize: payload.BatchSize,
			DryRun:    payload.DryRun,
		})
		if err != nil {
			return err
		}
		log.Printf("ingest result company=%d dry_run=%v reports=%d articles=%d skipped=%d chunks=%d indexed_points=%d batches=%d",
			job.CompanyID, payload.DryRun, result.Reports, result.Articles, result.ArticleSkipped,
			result.IndexedChunks, result.IndexedPoints, result.IndexBatches)

		if !payload.DryRun && graphWorkflow != nil && result.Articles > 0 {
			runGraphsync(ctx, graphWorkflow, job.CompanyID, since, result.Articles)
		}
		return nil
	}
}
