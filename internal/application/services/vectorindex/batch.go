package vectorindex

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

type KeyedVector struct {
	Key    string
	Vector []float32
}

type MetaSource interface {
	MetaByIDs(ctx context.Context, companyID int64, ids []int64) ([]Chunk, error)
}

type BatchIngestor struct {
	meta   MetaSource
	vector ports.VectorStore
	marker Marker
}

func NewBatchIngestor(meta MetaSource, vector ports.VectorStore, marker Marker) *BatchIngestor {
	return &BatchIngestor{meta: meta, vector: vector, marker: marker}
}

func FormatBatchKey(companyID, chunkID int64) string {
	return strconv.FormatInt(companyID, 10) + ":" + strconv.FormatInt(chunkID, 10)
}

func ParseBatchKey(key string) (int64, int64, error) {
	parts := strings.Split(key, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid batch key %q", key)
	}
	company, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || company <= 0 {
		return 0, 0, fmt.Errorf("invalid company in batch key %q", key)
	}
	chunk, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || chunk <= 0 {
		return 0, 0, fmt.Errorf("invalid chunk in batch key %q", key)
	}
	return company, chunk, nil
}

func (b *BatchIngestor) Ingest(ctx context.Context, opts Options, vectors []KeyedVector) (Result, error) {
	if opts.CollectionPrefix == "" {
		return Result{}, errors.New("collection prefix is required")
	}
	if len(vectors) == 0 {
		return Result{}, nil
	}

	vectorByCompanyChunk := make(map[int64]map[int64][]float32)
	companyOrder := make([]int64, 0)
	idsByCompany := make(map[int64][]int64)
	dim := 0
	for _, v := range vectors {
		company, chunk, err := ParseBatchKey(v.Key)
		if err != nil {
			return Result{}, err
		}
		if len(v.Vector) == 0 {
			return Result{}, fmt.Errorf("batch ingest: empty vector for key %s", v.Key)
		}
		if dim == 0 {
			dim = len(v.Vector)
		} else if len(v.Vector) != dim {
			return Result{}, fmt.Errorf("batch ingest: inconsistent vector dim %d != %d for key %s", len(v.Vector), dim, v.Key)
		}
		if _, ok := vectorByCompanyChunk[company]; !ok {
			vectorByCompanyChunk[company] = make(map[int64][]float32)
			companyOrder = append(companyOrder, company)
		}
		vectorByCompanyChunk[company][chunk] = v.Vector
		idsByCompany[company] = append(idsByCompany[company], chunk)
	}

	var result Result
	for _, company := range companyOrder {
		companyCtx := ctxkey.WithCompanyID(ctx, company)
		metaRows, err := b.meta.MetaByIDs(companyCtx, company, idsByCompany[company])
		if err != nil {
			return Result{}, err
		}
		metaMap := make(map[int64]Chunk, len(metaRows))
		for _, chunk := range metaRows {
			metaMap[chunk.ID] = chunk
		}

		points := make([]ports.VectorPoint, 0, len(idsByCompany[company]))
		refs := make([]ChunkRef, 0, len(idsByCompany[company]))
		for chunkID, vector := range vectorByCompanyChunk[company] {
			chunk, ok := metaMap[chunkID]
			if !ok {
				return Result{}, fmt.Errorf("batch ingest: no chunk metadata for key %s", FormatBatchKey(company, chunkID))
			}
			if chunk.CompanyID != company {
				return Result{}, fmt.Errorf("batch ingest: key company %d does not match chunk %d company %d", company, chunkID, chunk.CompanyID)
			}

			pointID := strconv.FormatInt(chunk.ID, 10)
			points = append(points, ports.VectorPoint{
				ID:     pointID,
				Vector: vector,
				Metadata: map[string]any{
					"company_id":       company,
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
				Dim:       len(vector),
			})
		}

		if len(points) == 0 {
			continue
		}
		collection := fmt.Sprintf("%s__%d", opts.CollectionPrefix, company)
		if err := b.vector.EnsureCollection(companyCtx, collection, dim); err != nil {
			return Result{}, err
		}
		if err := b.vector.Upsert(companyCtx, collection, points); err != nil {
			return Result{}, err
		}
		if err := b.marker.Mark(companyCtx, refs); err != nil {
			return Result{}, err
		}
		result.Points += len(points)
		result.Chunks += len(points)
		result.Batches++
	}
	return result, nil
}
