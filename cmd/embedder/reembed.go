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

func runReembed(ctx context.Context, deps commandDependencies, opts commandOptions) {
	if opts.company <= 0 {
		log.Fatal("reembed requires -company=<id> (re-embed is scoped to one tenant)")
	}
	resolvedProvider, modelName, embedder, err := embeddings.NewProvider(embeddingSettings(opts, deps.cfg))
	if err != nil {
		log.Fatalf("build embedder: %v", err)
	}
	reembedder := vectorindex.NewReembedder(
		mysqlkb.NewReportSource(deps.qdb),
		embedder,
		qdrant.NewStore(qdrant.Config{URL: deps.cfg.Qdrant.URL, APIKey: deps.cfg.Qdrant.APIKey}),
	)
	result, err := reembedder.Run(ctx, vectorindex.ReembedOptions{
		CompanyID:        opts.company,
		CollectionPrefix: opts.collection,
		ChunkRunes:       opts.chunkRunes,
		Limit:            opts.limit,
		Model:            modelName,
	})
	if err != nil {
		log.Fatalf("reembed: %v", err)
	}
	fmt.Printf("reembed complete: company=%d collection=%s dim=%d dimension=%s reports=%d chunks=%d points=%d provider=%s model=%s\n",
		opts.company, result.Collection, result.Dim, result.DimensionAction, result.Reports, result.Chunks, result.Points, resolvedProvider, modelName)
}
