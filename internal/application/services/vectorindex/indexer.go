package vectorindex

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

type Chunk struct {
	ID             int64
	ArticleID      int64
	CompanyID      int64
	SourceReportID int64
	ChunkIndex     int
	Content        string
}

type ChunkRef struct {
	ChunkID   int64
	VectorRef string
	Model     string
	Dim       int
}

type ChunkSource interface {
	Next(ctx context.Context, companyID int64, afterID int64, limit int) ([]Chunk, error)
}

type Marker interface {
	Mark(ctx context.Context, refs []ChunkRef) error
}

type Options struct {
	CompanyID        int64
	CollectionPrefix string
	BatchSize        int
	Limit            int
	Model            string
	DryRun           bool
}

type Result struct {
	Chunks              int
	Points              int
	Batches             int
	EstimatedInputRunes int
}

type Indexer struct {
	source ChunkSource
	embed  ports.EmbeddingProvider
	vector ports.VectorStore
	marker Marker
}

func NewIndexer(source ChunkSource, embed ports.EmbeddingProvider, vector ports.VectorStore, marker Marker) *Indexer {
	return &Indexer{source: source, embed: embed, vector: vector, marker: marker}
}

func (i *Indexer) Run(ctx context.Context, opts Options) (Result, error) {
	if err := validateOptions(opts); err != nil {
		return Result{}, err
	}

	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 64
	}

	var result Result
	var afterID int64
	// track which collections we've already ensured
	ensured := make(map[string]bool)

	for {
		if opts.Limit > 0 && result.Chunks >= opts.Limit {
			break
		}

		readLimit := batchSize
		if opts.Limit > 0 && result.Chunks+readLimit > opts.Limit {
			readLimit = opts.Limit - result.Chunks
		}
		chunks, err := i.source.Next(ctx, opts.CompanyID, afterID, readLimit)
		if err != nil {
			return Result{}, err
		}
		if len(chunks) == 0 {
			break
		}

		if opts.CompanyID > 0 {
			for _, chunk := range chunks {
				if chunk.CompanyID != opts.CompanyID {
					return Result{}, fmt.Errorf("indexer: chunk %d belongs to company %d, not %d", chunk.ID, chunk.CompanyID, opts.CompanyID)
				}
			}
		}

		if opts.DryRun {
			for _, chunk := range chunks {
				result.EstimatedInputRunes += len([]rune(chunk.Content))
			}
			afterID = chunks[len(chunks)-1].ID
			result.Batches++
			result.Chunks += len(chunks)
			continue
		}

		points, err := i.indexChunks(ctx, opts, chunks, ensured)
		if err != nil {
			return Result{}, err
		}
		result.Points += points

		afterID = chunks[len(chunks)-1].ID
		result.Batches++
		result.Chunks += len(chunks)
	}

	return result, nil
}

func (i *Indexer) embedChunks(ctx context.Context, opts Options, chunks []Chunk, companyID int64) ([]ports.VectorPoint, []ChunkRef, error) {
	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		texts = append(texts, chunk.Content)
	}

	vectors, err := i.embed.Embed(ctx, texts)
	if err != nil {
		return nil, nil, err
	}
	if len(vectors) != len(chunks) {
		return nil, nil, fmt.Errorf("embedding count %d does not match chunk count %d", len(vectors), len(chunks))
	}

	points := make([]ports.VectorPoint, 0, len(chunks))
	refs := make([]ChunkRef, 0, len(chunks))
	for idx, chunk := range chunks {
		pointID := strconv.FormatInt(chunk.ID, 10)
		points = append(points, ports.VectorPoint{
			ID:     pointID,
			Vector: vectors[idx],
			Metadata: map[string]any{
				"company_id":       companyID,
				"kb_article_id":    chunk.ArticleID,
				"kb_chunk_id":      chunk.ID,
				"source_report_id": chunk.SourceReportID,
				"chunk_index":      chunk.ChunkIndex,
				"chunk_kind":       "child",
				"is_golden":        false,
				"base_score":       1.0,
			},
		})
		refs = append(refs, ChunkRef{
			ChunkID:   chunk.ID,
			VectorRef: pointID,
			Model:     opts.Model,
			Dim:       i.embed.Dim(),
		})
	}
	return points, refs, nil
}

func (i *Indexer) indexChunks(ctx context.Context, opts Options, chunks []Chunk, ensured map[string]bool) (int, error) {
	byCompany := make(map[int64][]Chunk)
	for _, chunk := range chunks {
		byCompany[chunk.CompanyID] = append(byCompany[chunk.CompanyID], chunk)
	}

	var indexed int
	for cid, companyChunks := range byCompany {
		collection := fmt.Sprintf("%s__%d", opts.CollectionPrefix, cid)
		if !ensured[collection] {
			if err := i.vector.EnsureCollection(ctx, collection, i.embed.Dim()); err != nil {
				return 0, err
			}
			ensured[collection] = true
		}

		points, refs, err := i.embedChunks(ctx, opts, companyChunks, cid)
		if err != nil {
			return 0, err
		}
		if err := i.vector.Upsert(ctx, collection, points); err != nil {
			return 0, err
		}
		markCtx := ctxkey.WithCompanyID(ctx, cid)
		if err := i.marker.Mark(markCtx, refs); err != nil {
			return 0, err
		}
		indexed += len(points)
	}
	return indexed, nil
}

func validateOptions(opts Options) error {
	if opts.CollectionPrefix == "" {
		return errors.New("collection prefix is required")
	}
	return nil
}
