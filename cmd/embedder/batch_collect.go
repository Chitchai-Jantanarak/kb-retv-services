package main

import (
	"context"
	"fmt"
	"log"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/application/services/vectorindex"
	"github.com/my/app/internal/infra/qdrant"
	mysqlkb "github.com/my/app/internal/repositories/kb/mysql"
)

func runBatchCollect(ctx context.Context, deps commandDependencies, opts commandOptions) {
	_, _, batchEmbedder, err := embeddings.NewBatchEmbedder(embeddingSettings(opts, deps.cfg))
	if err != nil {
		log.Fatalf("build batch embedder: %v", err)
	}
	repo := mysqlkb.NewEmbeddingBatchRepo(deps.db)
	jobs, err := loadBatchJobs(ctx, repo, opts.batchJob)
	if err != nil {
		log.Fatalf("load batches: %v", err)
	}
	if len(jobs) == 0 {
		fmt.Println("no active batches to collect")
		return
	}
	ingestor := vectorindex.NewBatchIngestor(
		mysqlkb.NewChunkSourceForModel(deps.qdb, batchEmbedder.Model()),
		qdrant.NewStore(qdrant.Config{URL: deps.cfg.Qdrant.URL, APIKey: deps.cfg.Qdrant.APIKey}),
		mysqlkb.NewEmbeddingMarker(deps.qdb),
	)
	for _, job := range jobs {
		poll, err := batchEmbedder.Poll(ctx, job.BatchName)
		if err != nil {
			_ = repo.UpdateState(ctx, job.BatchName, pollState(poll.State), err.Error())
			log.Printf("batch %s failed: %v", job.BatchName, err)
			continue
		}
		if !poll.Done {
			_ = repo.UpdateState(ctx, job.BatchName, pollState(poll.State), "")
			fmt.Printf("batch %s: %s (not ready)\n", job.BatchName, poll.State)
			continue
		}
		res, err := ingestor.Ingest(ctx, vectorindex.Options{CollectionPrefix: opts.collection, Model: job.Model}, toKeyedVectors(poll.Results))
		if err != nil {
			_ = repo.UpdateState(ctx, job.BatchName, pollState(poll.State), err.Error())
			log.Printf("ingest batch %s failed; left active for retry: %v", job.BatchName, err)
			continue
		}
		if err := repo.MarkIngested(ctx, job.BatchName, res.Points); err != nil {
			log.Printf("batch %s ingested points=%d but state not recorded (will re-collect): %v", job.BatchName, res.Points, err)
			continue
		}
		if len(poll.Failed) > 0 {
			log.Printf("batch %s: %d key(s) failed at provider, left un-embedded for re-queue", job.BatchName, len(poll.Failed))
		}
		fmt.Printf("batch %s ingested: points=%d companies=%d collection=%s model=%s\n",
			job.BatchName, res.Points, res.Batches, opts.collection, job.Model)
	}
}
