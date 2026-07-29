package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/application/services/vectorindex"
	mysqlkb "github.com/my/app/internal/repositories/kb/mysql"
)

func toKeyedVectors(results []embeddings.BatchEmbedResult) []vectorindex.KeyedVector {
	kvs := make([]vectorindex.KeyedVector, 0, len(results))
	for _, result := range results {
		kvs = append(kvs, vectorindex.KeyedVector{Key: result.Key, Vector: result.Values})
	}
	return kvs
}

func parseCompanies(csv string, single int64) []int64 {
	if strings.TrimSpace(csv) != "" {
		var ids []int64
		for part := range strings.SplitSeq(csv, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err == nil && id > 0 {
				ids = append(ids, id)
			}
		}
		return ids
	}
	if single > 0 {
		return []int64{single}
	}
	return nil
}

func collectBatchRequests(
	ctx context.Context,
	source *mysqlkb.ChunkSource,
	companyIDs []int64,
	batchSize int,
	limit int,
) ([]embeddings.BatchEmbedRequest, error) {
	if batchSize <= 0 {
		batchSize = 500
	}
	var reqs []embeddings.BatchEmbedRequest
	for _, companyID := range companyIDs {
		var afterID int64
		for {
			if limit > 0 && len(reqs) >= limit {
				return reqs[:limit], nil
			}
			chunks, err := source.Next(ctx, companyID, afterID, batchSize)
			if err != nil {
				return nil, err
			}
			if len(chunks) == 0 {
				break
			}
			for _, chunk := range chunks {
				reqs = append(reqs, embeddings.BatchEmbedRequest{
					Key:  vectorindex.FormatBatchKey(companyID, chunk.ID),
					Text: chunk.Content,
				})
			}
			afterID = chunks[len(chunks)-1].ID
		}
	}
	return reqs, nil
}

func chunkRequests(reqs []embeddings.BatchEmbedRequest, maxPerJob int) [][]embeddings.BatchEmbedRequest {
	if maxPerJob <= 0 || len(reqs) <= maxPerJob {
		return [][]embeddings.BatchEmbedRequest{reqs}
	}
	var jobs [][]embeddings.BatchEmbedRequest
	for start := 0; start < len(reqs); start += maxPerJob {
		end := min(start+maxPerJob, len(reqs))
		jobs = append(jobs, reqs[start:end])
	}
	return jobs
}

func loadBatchJobs(ctx context.Context, repo *mysqlkb.EmbeddingBatchRepo, name string) ([]mysqlkb.EmbeddingBatch, error) {
	if name != "" {
		job, err := repo.GetByName(ctx, name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("no batch named %q", name)
			}
			return nil, err
		}
		return []mysqlkb.EmbeddingBatch{job}, nil
	}
	return repo.ListActive(ctx)
}

func pollState(state string) string {
	if strings.TrimSpace(state) == "" {
		return "POLL_ERROR"
	}
	return state
}
