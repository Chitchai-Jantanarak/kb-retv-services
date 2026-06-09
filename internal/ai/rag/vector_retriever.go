package rag

import (
	"context"
	"fmt"
	"strconv"

	"github.com/my/app/internal/domain/kb"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

type VectorRetrieverConfig struct {
	Embedder         ports.EmbeddingProvider
	Vector           ports.VectorStore
	Chunks           ports.KnowledgeChunkRepository
	CollectionPrefix string
}

type VectorRetriever struct {
	embedder         ports.EmbeddingProvider
	vector           ports.VectorStore
	chunks           ports.KnowledgeChunkRepository
	collectionPrefix string
}

func NewVectorRetriever(cfg VectorRetrieverConfig) *VectorRetriever {
	return &VectorRetriever{
		embedder:         cfg.Embedder,
		vector:           cfg.Vector,
		chunks:           cfg.Chunks,
		collectionPrefix: cfg.CollectionPrefix,
	}
}

func (r *VectorRetriever) Retrieve(ctx context.Context, query Query, meta Meta) ([]Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.embedder == nil || r.vector == nil || r.chunks == nil {
		return nil, nil
	}

	cid := vectorCompanyID(ctx, query)
	if cid <= 0 {
		return nil, nil
	}

	collection := fmt.Sprintf("%s__%d", vectorCollectionPrefix(r.collectionPrefix), cid)

	vectors, err := r.embedder.Embed(ctx, []string{query.Text})
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("query embedding count %d does not match input count 1", len(vectors))
	}

	limit := vectorLimit(query)
	searchLimit := limit * 5
	hits, err := r.vector.Search(ctx, ports.VectorSearch{
		Collection: collection,
		Query:      vectors[0],
		Limit:      searchLimit,
	})
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return nil, nil
	}

	ids, scores := vectorHitIDs(hits)
	if len(ids) == 0 {
		return nil, nil
	}

	found, err := r.chunks.ChunksByIDs(ctx, kb.ChunkQuery{
		IDs:       ids,
		CompanyID: cid,
	})
	if err != nil {
		return nil, err
	}

	return vectorCandidates(ids, scores, found, limit), nil
}

func (r *VectorRetriever) RetrievalCost() RetrievalCost {
	return RetrievalCostExpensive
}

func vectorCompanyID(ctx context.Context, query Query) int64 {
	if query.CompanyID > 0 {
		return query.CompanyID
	}
	if v, ok := ctxkey.CompanyID(ctx); ok {
		return v
	}
	return 0
}

func vectorCollectionPrefix(prefix string) string {
	if prefix == "" {
		return "kb_chunks"
	}
	return prefix
}

func vectorLimit(query Query) int {
	if query.Limit > 0 {
		return query.Limit
	}
	return 5
}

func vectorHitIDs(hits []ports.VectorHit) ([]string, map[string]float64) {
	ids := make([]string, 0, len(hits))
	scores := make(map[string]float64, len(hits))
	for _, hit := range hits {
		id := chunkIDFromHit(hit)
		if id == "" {
			continue
		}
		ids = append(ids, id)
		scores[id] = float64(hit.Score)
	}
	return ids, scores
}

func vectorCandidates(ids []string, scores map[string]float64, found []kb.Chunk, limit int) []Candidate {
	byID := make(map[string]kb.Chunk, len(found))
	for _, chunk := range found {
		byID[chunk.ID] = chunk
	}
	candidates := make([]Candidate, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		chunk, ok := byID[id]
		if !ok {
			continue
		}
		candidateID := chunk.ArticleID + ":" + chunk.ID
		if _, ok := seen[candidateID]; ok {
			continue
		}
		seen[candidateID] = struct{}{}
		candidates = append(candidates, Candidate{
			ID:        candidateID,
			ArticleID: chunk.ArticleID,
			ChunkID:   chunk.ID,
			Title:     chunk.Title,
			Content:   chunk.Content,
			Source:    "vector",
			Score:     scores[id],
		})
		if len(candidates) == limit {
			break
		}
	}
	return candidates
}

func chunkIDFromHit(hit ports.VectorHit) string {
	if value, ok := hit.Metadata["kb_chunk_id"]; ok {
		return stringifyID(value)
	}
	return hit.ID
}

func stringifyID(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return fmt.Sprint(v)
	}
}

var _ Retriever = (*VectorRetriever)(nil)
var _ CostAwareRetriever = (*VectorRetriever)(nil)
