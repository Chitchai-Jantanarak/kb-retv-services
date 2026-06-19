package vectorindex

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/my/app/internal/application/services/kbbootstrap"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

type ReportReader interface {
	Reports(ctx context.Context, companyID int64, limit int) ([]kbbootstrap.ReportRecord, error)
}

type ReembedOptions struct {
	CompanyID        int64
	CollectionPrefix string
	ChunkRunes       int
	Limit            int
	Model            string
}

type ReembedResult struct {
	Collection      string
	DimensionAction DimensionAction
	Dim             int
	Reports         int
	Chunks          int
	Points          int
}

type Reembedder struct {
	reports ReportReader
	embed   ports.EmbeddingProvider
	vector  ReembedVectorStore
}

type ReembedVectorStore interface {
	CollectionManager
	Upsert(ctx context.Context, collection string, points []ports.VectorPoint) error
}

func NewReembedder(reports ReportReader, embed ports.EmbeddingProvider, vector ReembedVectorStore) *Reembedder {
	return &Reembedder{reports: reports, embed: embed, vector: vector}
}

func (r *Reembedder) Run(ctx context.Context, opts ReembedOptions) (ReembedResult, error) {
	if opts.CompanyID <= 0 {
		return ReembedResult{}, errors.New("reembed: company id must be positive")
	}
	if opts.CollectionPrefix == "" {
		return ReembedResult{}, errors.New("reembed: collection prefix is required")
	}

	dim := r.embed.Dim()
	if dim <= 0 {
		return ReembedResult{}, errors.New("reembed: embedder reported a non-positive dimension")
	}

	ctx = ctxkey.WithCompanyID(ctx, opts.CompanyID)
	collection := fmt.Sprintf("%s__%d", opts.CollectionPrefix, opts.CompanyID)

	action, err := ReconcileDimension(ctx, r.vector, collection, dim)
	if err != nil {
		return ReembedResult{}, err
	}

	reports, err := r.reports.Reports(ctx, opts.CompanyID, opts.Limit)
	if err != nil {
		return ReembedResult{}, fmt.Errorf("read reports: %w", err)
	}

	result := ReembedResult{
		Collection:      collection,
		DimensionAction: action,
		Dim:             dim,
	}

	for _, report := range reports {
		if report.CompanyID != opts.CompanyID {
			return ReembedResult{}, fmt.Errorf("reembed: report %d belongs to company %d, not %d", report.ID, report.CompanyID, opts.CompanyID)
		}
		result.Reports++

		body := kbbootstrap.LimitUTF8Bytes(kbbootstrap.BuildArticleBody(report), kbbootstrap.MaxArticleBodyBytes)
		chunks := kbbootstrap.ChunkText(body, opts.ChunkRunes)
		if len(chunks) == 0 {
			continue
		}

		vectors, err := r.embed.Embed(ctx, chunks)
		if err != nil {
			return ReembedResult{}, fmt.Errorf("embed report %d: %w", report.ID, err)
		}
		if len(vectors) != len(chunks) {
			return ReembedResult{}, fmt.Errorf("reembed: report %d got %d vectors for %d chunks", report.ID, len(vectors), len(chunks))
		}

		points := make([]ports.VectorPoint, 0, len(chunks))
		for idx, chunk := range chunks {
			points = append(points, ports.VectorPoint{
				ID:     reembedPointID(report.ID, idx),
				Vector: vectors[idx],
				Metadata: map[string]any{
					"company_id":       opts.CompanyID,
					"source_report_id": report.ID,
					"chunk_index":      idx,
					"chunk_kind":       "child",
					"model":            opts.Model,
					"content":          chunk,
				},
			})
		}
		if err := r.vector.Upsert(ctx, collection, points); err != nil {
			return ReembedResult{}, fmt.Errorf("upsert report %d: %w", report.ID, err)
		}
		result.Chunks += len(chunks)
		result.Points += len(points)
	}

	return result, nil
}

func reembedPointID(reportID int64, chunkIndex int) string {
	return strconv.FormatInt(reportID*1000+int64(chunkIndex), 10)
}
