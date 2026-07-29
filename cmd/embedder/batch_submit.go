package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/my/app/internal/ai/embeddings"
	mysqlkb "github.com/my/app/internal/repositories/kb/mysql"
)

func runBatchSubmit(ctx context.Context, deps commandDependencies, opts commandOptions) {
	resolvedProvider, _, batchEmbedder, err := embeddings.NewBatchEmbedder(embeddingSettings(opts, deps.cfg))
	if err != nil {
		log.Fatalf("build batch embedder: %v", err)
	}
	companyIDs := parseCompanies(opts.companies, opts.company)
	if len(companyIDs) == 0 {
		log.Fatal("embed-batch-submit requires -company=<id> or -companies=<id,id> (cross-tenant data is sharded; an explicit company set is required)")
	}
	source := mysqlkb.NewChunkSourceForModel(deps.qdb, batchEmbedder.Model())
	reqs, err := collectBatchRequests(ctx, source, companyIDs, opts.batchSize, opts.limit)
	if err != nil {
		log.Fatalf("gather chunks: %v", err)
	}
	if len(reqs) == 0 {
		fmt.Printf("nothing to embed: no un-embedded chunks for companies %v (model=%s)\n", companyIDs, batchEmbedder.Model())
		return
	}

	repo := mysqlkb.NewEmbeddingBatchRepo(deps.db)
	displayBase := fmt.Sprintf("kb-%s-%s-%d", resolvedProvider, batchEmbedder.Model(), time.Now().Unix())
	jobs := chunkRequests(reqs, opts.maxPerJob)
	created, submitted := 0, 0
	var orphaned []string
	for idx, chunk := range jobs {
		displayName := displayBase
		if len(jobs) > 1 {
			displayName = fmt.Sprintf("%s-p%d", displayBase, idx)
		}
		jobName, err := batchEmbedder.Submit(ctx, displayName, chunk)
		if err != nil {
			log.Printf("submit job %d/%d failed (%d created so far): %v", idx+1, len(jobs), created, err)
			break
		}
		if err := repo.Insert(ctx, mysqlkb.EmbeddingBatch{
			BatchName:    jobName,
			Provider:     resolvedProvider,
			Model:        batchEmbedder.Model(),
			Dimensions:   batchEmbedder.Dim(),
			DisplayName:  displayName,
			State:        "JOB_STATE_PENDING",
			RequestCount: len(chunk),
		}); err != nil {
			orphaned = append(orphaned, jobName)
			log.Printf("WARNING batch job %s submitted but not recorded: %v — collect or re-insert manually before it expires", jobName, err)
			continue
		}
		created++
		submitted += len(chunk)
		fmt.Printf("[%d/%d] %s (%d reqs)\n", idx+1, len(jobs), jobName, len(chunk))
		if idx+1 < len(jobs) {
			time.Sleep(3 * time.Second)
		}
	}
	fmt.Printf("submitted %d/%d jobs, %d/%d requests, companies=%v model=%s dim=%d\n",
		created, len(jobs), submitted, len(reqs), companyIDs, batchEmbedder.Model(), batchEmbedder.Dim())
	if len(orphaned) > 0 {
		fmt.Printf("WARNING %d submitted job(s) not recorded (recover manually): %v\n", len(orphaned), orphaned)
	}
	fmt.Println("collect with: -task=embed-batch-collect (drains all active jobs)")
}
