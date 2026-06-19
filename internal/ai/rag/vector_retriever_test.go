package rag

import (
	"context"
	"math"
	"testing"

	"github.com/my/app/internal/domain/kb"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

func TestVectorRetrieverEmbedsSearchesAndMapsChunks(t *testing.T) {
	embedder := &fakeVectorEmbedder{vector: []float32{0.1, 0.2}}
	store := &fakeVectorSearchStore{
		hits: []ports.VectorHit{
			{ID: "11", Score: 0.82, Metadata: map[string]any{"kb_chunk_id": float64(11)}},
			{ID: "12", Score: 0.72, Metadata: map[string]any{"kb_chunk_id": "12"}},
		},
	}
	chunks := &fakeChunkLookup{
		chunks: []kb.Chunk{
			{ID: "12", ArticleID: "2", CompanyID: 3, Title: "Late candidate", Content: "second"},
			{ID: "11", ArticleID: "1", CompanyID: 3, Title: "Printer offline", Content: "first"},
		},
	}
	retriever := NewVectorRetriever(VectorRetrieverConfig{
		Embedder:         embedder,
		Vector:           store,
		Chunks:           chunks,
		CollectionPrefix: "kb_chunks",
	})

	ctx := ctxkey.WithCompanyID(context.Background(), 3)
	candidates, err := retriever.Retrieve(ctx, Query{
		CompanyID: 3,
		Text:      "printer offline",
		Limit:     2,
	}, Meta{})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}

	if len(embedder.texts) != 1 || embedder.texts[0] != "printer offline" {
		t.Fatalf("embedded texts = %#v", embedder.texts)
	}
	if store.search.Collection != "kb_chunks__3" {
		t.Fatalf("collection = %q, want kb_chunks__3", store.search.Collection)
	}
	if len(store.search.Filter) != 0 {
		t.Fatalf("filter should be empty, got %v", store.search.Filter)
	}
	if chunks.query.CompanyID != 3 {
		t.Fatalf("lookup company_id = %d, want 3", chunks.query.CompanyID)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(candidates))
	}
	if candidates[0].ID != "1:11" {
		t.Fatalf("first ID = %q, want 1:11", candidates[0].ID)
	}
	if candidates[0].Title != "Printer offline" {
		t.Fatalf("first title = %q, want Printer offline", candidates[0].Title)
	}
	if math.Abs(candidates[0].Score-0.82) > 0.0001 {
		t.Fatalf("first score = %v, want 0.82", candidates[0].Score)
	}
	if candidates[0].Source != "vector" {
		t.Fatalf("source = %q, want vector", candidates[0].Source)
	}
}

func TestVectorRetrieverFansOutAcrossCoverage(t *testing.T) {
	embedder := &fakeVectorEmbedder{vector: []float32{0.1, 0.2}}
	store := &fakeVectorSearchStore{
		hits: []ports.VectorHit{
			{ID: "11", Score: 0.82, Metadata: map[string]any{"kb_chunk_id": float64(11)}},
		},
	}
	chunks := &fakeChunkLookup{
		chunks: []kb.Chunk{
			{ID: "11", ArticleID: "1", CompanyID: 3, Title: "Printer offline", Content: "first"},
		},
	}
	retriever := NewVectorRetriever(VectorRetrieverConfig{
		Embedder:         embedder,
		Vector:           store,
		Chunks:           chunks,
		CollectionPrefix: "kb_chunks",
	})

	ctx := ctxkey.WithCompanyID(context.Background(), 3)
	_, err := retriever.Retrieve(ctx, Query{
		CompanyID: 3,
		Coverage:  []int64{3, 8},
		Text:      "printer offline",
		Limit:     2,
	}, Meta{})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}

	if len(store.collections) != 2 || store.collections[0] != "kb_chunks__3" || store.collections[1] != "kb_chunks__8" {
		t.Fatalf("collections searched = %v, want [kb_chunks__3 kb_chunks__8]", store.collections)
	}
	if len(chunks.query.Coverage) != 2 || chunks.query.Coverage[0] != 3 || chunks.query.Coverage[1] != 8 {
		t.Fatalf("lookup coverage = %v, want [3 8]", chunks.query.Coverage)
	}
}

type fakeVectorEmbedder struct {
	texts  []string
	vector []float32
}

func (e *fakeVectorEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e.texts = append(e.texts, texts...)
	return [][]float32{e.vector}, nil
}

func (e *fakeVectorEmbedder) Dim() int {
	return len(e.vector)
}

type fakeVectorSearchStore struct {
	search      ports.VectorSearch
	collections []string
	hits        []ports.VectorHit
}

func (s *fakeVectorSearchStore) EnsureCollection(ctx context.Context, name string, dim int) error {
	return nil
}

func (s *fakeVectorSearchStore) Upsert(ctx context.Context, collection string, points []ports.VectorPoint) error {
	return nil
}

func (s *fakeVectorSearchStore) Search(ctx context.Context, q ports.VectorSearch) ([]ports.VectorHit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.search = q
	s.collections = append(s.collections, q.Collection)
	return s.hits, nil
}

func (s *fakeVectorSearchStore) Delete(ctx context.Context, collection string, ids []string) error {
	return nil
}

type fakeChunkLookup struct {
	query  kb.ChunkQuery
	chunks []kb.Chunk
}

func (l *fakeChunkLookup) ChunksByIDs(ctx context.Context, query kb.ChunkQuery) ([]kb.Chunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.query = query
	return l.chunks, nil
}
