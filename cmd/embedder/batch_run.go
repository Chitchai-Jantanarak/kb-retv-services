package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/application/services/vectorindex"
	"github.com/my/app/internal/infra/qdrant"
	mysqlkb "github.com/my/app/internal/repositories/kb/mysql"
)

func runBatch(ctx context.Context, deps commandDependencies, opts commandOptions) {
	resolvedProvider, _, batchEmbedder, err := embeddings.NewBatchEmbedder(embeddingSettings(opts, deps.cfg))
	if err != nil {
		log.Fatalf("build batch embedder: %v", err)
	}
	companyIDs := parseCompanies(opts.companies, opts.company)
	if len(companyIDs) == 0 {
		log.Fatal("embed-batch-run requires -company=<id> or -companies=<id,id>")
	}
	repo := mysqlkb.NewEmbeddingBatchRepo(deps.db)
	source := mysqlkb.NewChunkSourceForModel(deps.qdb, batchEmbedder.Model())
	ingestor := vectorindex.NewBatchIngestor(
		source,
		qdrant.NewStore(qdrant.Config{URL: deps.cfg.Qdrant.URL, APIKey: deps.cfg.Qdrant.APIKey}),
		mysqlkb.NewEmbeddingMarker(deps.qdb),
	)

	active, err := repo.ListActive(ctx)
	if err != nil {
		log.Fatalf("list active batches: %v", err)
	}
	inFlight := 0
	for _, job := range active {
		poll, err := batchEmbedder.Poll(ctx, job.BatchName)
		if err != nil {
			_ = repo.UpdateState(ctx, job.BatchName, pollState(poll.State), err.Error())
			log.Printf("batch %s %s; its chunks return to the queue", job.BatchName, pollState(poll.State))
			continue
		}
		if !poll.Done {
			_ = repo.UpdateState(ctx, job.BatchName, pollState(poll.State), "")
			inFlight++
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
		fmt.Printf("ingested %s: points=%d\n", job.BatchName, res.Points)
	}

	if inFlight > 0 {
		fmt.Printf("%d job(s) still in flight; pool busy — re-run after they finish\n", inFlight)
		return
	}

	reqs, err := collectBatchRequests(ctx, source, companyIDs, opts.batchSize, opts.waveSize)
	if err != nil {
		log.Fatalf("gather chunks: %v", err)
	}
	if len(reqs) == 0 {
		fmt.Printf("all chunks embedded for companies %v (model=%s) — nothing to do\n", companyIDs, batchEmbedder.Model())
		return
	}
	displayName := fmt.Sprintf("kb-%s-%s-%d", resolvedProvider, batchEmbedder.Model(), time.Now().Unix())
	jobName, err := batchEmbedder.Submit(ctx, displayName, reqs)
	if err != nil {
		fmt.Printf("wave not submitted (%d chunks pending): %v\nenqueue pool likely busy — re-run later\n", len(reqs), err)
		return
	}
	if err := repo.Insert(ctx, mysqlkb.EmbeddingBatch{
		BatchName:    jobName,
		Provider:     resolvedProvider,
		Model:        batchEmbedder.Model(),
		Dimensions:   batchEmbedder.Dim(),
		DisplayName:  displayName,
		State:        "JOB_STATE_PENDING",
		RequestCount: len(reqs),
	}); err != nil {
		log.Printf("WARNING wave %s submitted but not recorded: %v — collect or re-insert manually before it expires", jobName, err)
		return
	}
	fmt.Printf("wave submitted %s: %d chunks (companies=%v model=%s). Re-run after it completes to continue.\n",
		jobName, len(reqs), companyIDs, batchEmbedder.Model())
}
