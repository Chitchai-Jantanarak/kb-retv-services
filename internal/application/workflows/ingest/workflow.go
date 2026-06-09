package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/my/app/internal/application/services/kbbootstrap"
	"github.com/my/app/internal/application/services/vectorindex"
)

type Options struct {
	Limit     int
	BatchSize int
	DryRun    bool
}

type Result struct {
	Reports        int
	Articles       int
	ArticleSkipped int
	ArticleChunks  int
	IndexedChunks  int
	IndexedPoints  int
	IndexBatches   int
}

type Workflow struct {
	bootstrapper     *kbbootstrap.Bootstrapper
	indexer          *vectorindex.Indexer
	collectionPrefix string
	model            string
	chunkRunes       int
}

type Config struct {
	Bootstrapper     *kbbootstrap.Bootstrapper
	Indexer          *vectorindex.Indexer
	CollectionPrefix string
	Model            string
	ChunkRunes       int
}

func New(cfg Config) (*Workflow, error) {
	if cfg.Bootstrapper == nil {
		return nil, errors.New("ingest: bootstrapper is required")
	}
	if cfg.Indexer == nil {
		return nil, errors.New("ingest: indexer is required")
	}
	if cfg.CollectionPrefix == "" {
		cfg.CollectionPrefix = "kb_chunks"
	}
	return &Workflow{
		bootstrapper:     cfg.Bootstrapper,
		indexer:          cfg.Indexer,
		collectionPrefix: cfg.CollectionPrefix,
		model:            cfg.Model,
		chunkRunes:       cfg.ChunkRunes,
	}, nil
}

func (w *Workflow) Run(ctx context.Context, companyID int64, opts Options) (Result, error) {
	if companyID <= 0 {
		return Result{}, errors.New("ingest: company_id must be positive")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	bootRes, err := w.bootstrapper.Run(ctx, kbbootstrap.Options{
		CompanyID:  companyID,
		Limit:      opts.Limit,
		ChunkRunes: w.chunkRunes,
	})
	if err != nil {
		return Result{}, fmt.Errorf("ingest: bootstrap: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	idxRes, err := w.indexer.Run(ctx, vectorindex.Options{
		CompanyID:        companyID,
		CollectionPrefix: w.collectionPrefix,
		BatchSize:        opts.BatchSize,
		Limit:            opts.Limit,
		Model:            w.model,
		DryRun:           opts.DryRun,
	})
	if err != nil {
		return Result{}, fmt.Errorf("ingest: index: %w", err)
	}

	return Result{
		Reports:        bootRes.Reports,
		Articles:       bootRes.Articles,
		ArticleSkipped: bootRes.Skipped,
		ArticleChunks:  bootRes.Chunks,
		IndexedChunks:  idxRes.Chunks,
		IndexedPoints:  idxRes.Points,
		IndexBatches:   idxRes.Batches,
	}, nil
}
