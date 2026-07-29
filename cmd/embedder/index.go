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

func runIndexKB(ctx context.Context, deps commandDependencies, opts commandOptions) {
	resolvedProvider, modelName, embedder, err := embeddings.NewProvider(embeddingSettings(opts, deps.cfg))
	if err != nil {
		log.Fatalf("build embedder: %v", err)
	}
	indexer := vectorindex.NewIndexer(
		mysqlkb.NewChunkSourceForModel(deps.qdb, modelName),
		embedder,
		qdrant.NewStore(qdrant.Config{URL: deps.cfg.Qdrant.URL, APIKey: deps.cfg.Qdrant.APIKey}),
		mysqlkb.NewEmbeddingMarker(deps.qdb),
	)
	if opts.company == 0 {
		log.Println("warning: -company=0 scans chunks across every company; pass -company=<id> to scope")
	}
	limit := effectiveLimit(opts.limit, opts.maxChunks, deps.cfg.Embedding.RefreshMaxChunks)
	if !opts.dryRun && !opts.allowLargeRemoteRun && isRemoteProvider(resolvedProvider) {
		if limit == 0 || limit > deps.cfg.Embedding.LargeRunThreshold {
			log.Fatalf("refusing large remote embedding run (provider=%s limit=%d threshold=%d); pass -allow-large-remote-run, a smaller -max-chunks/-limit, or -dry-run first",
				resolvedProvider, limit, deps.cfg.Embedding.LargeRunThreshold)
		}
	}
	result, err := indexer.Run(ctx, vectorindex.Options{
		CompanyID:        opts.company,
		CollectionPrefix: opts.collection,
		BatchSize:        opts.batchSize,
		Limit:            limit,
		Model:            modelName,
		DryRun:           opts.dryRun,
	})
	if err != nil {
		log.Fatalf("index kb: %v", err)
	}
	if opts.dryRun {
		tokens := result.EstimatedInputRunes / 4
		costUSD := float64(tokens) / 1_000_000 * 0.15
		fmt.Printf("kb index dry-run: chunks=%d batches=%d estimated_runes=%d estimated_tokens~=%d estimated_cost_usd~=%.4f (estimate only, gemini standard embedding)\n",
			result.Chunks, result.Batches, result.EstimatedInputRunes, tokens, costUSD)
		return
	}
	fmt.Printf("kb index complete: chunks=%d points=%d batches=%d collection=%s model=%s provider=%s dim=%d\n",
		result.Chunks, result.Points, result.Batches, opts.collection, modelName, resolvedProvider, embedder.Dim())
}

func effectiveLimit(limit, maxChunks, configMax int) int {
	if limit > 0 {
		return limit
	}
	if maxChunks > 0 {
		return maxChunks
	}
	return configMax
}

func isRemoteProvider(provider string) bool {
	switch provider {
	case "hash", "ollama", "local":
		return false
	default:
		return true
	}
}
