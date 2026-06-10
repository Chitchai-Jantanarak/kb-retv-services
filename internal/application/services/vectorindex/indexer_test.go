package vectorindex

import (
	"context"
	"testing"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

func TestIndexerEmbedsUpsertsAndMarksChunks(t *testing.T) {
	source := &fakeChunkSource{
		chunks: []Chunk{
			{ID: 10, ArticleID: 1, CompanyID: 3, SourceReportID: 100, ChunkIndex: 0, Content: "printer offline"},
			{ID: 11, ArticleID: 1, CompanyID: 3, SourceReportID: 100, ChunkIndex: 1, Content: "network queue"},
		},
	}
	embedder := fakeEmbedder{dim: 3}
	vectorStore := &fakeVectorStore{}
	marker := &fakeMarker{}
	indexer := NewIndexer(source, embedder, vectorStore, marker)

	result, err := indexer.Run(context.Background(), Options{
		CollectionPrefix: "kb_chunks",
		BatchSize:        10,
		Limit:            10,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Chunks != 2 {
		t.Fatalf("Chunks = %d, want 2", result.Chunks)
	}
	if result.Points != 2 {
		t.Fatalf("Points = %d, want 2", result.Points)
	}
	if vectorStore.collection != "kb_chunks__3" {
		t.Fatalf("collection = %q, want kb_chunks__3", vectorStore.collection)
	}
	if len(vectorStore.points) != 2 {
		t.Fatalf("points = %d, want 2", len(vectorStore.points))
	}
	if vectorStore.points[0].ID != "10" {
		t.Fatalf("point ID = %q, want 10", vectorStore.points[0].ID)
	}
	if vectorStore.points[0].Metadata["company_id"] != int64(3) {
		t.Fatalf("company_id payload = %v, want 3", vectorStore.points[0].Metadata["company_id"])
	}
	if marker.refs[10] != "10" {
		t.Fatalf("marker ref = %q, want 10", marker.refs[10])
	}
}

func TestIndexerDryRunEstimatesInputRunes(t *testing.T) {
	source := &fakeChunkSource{
		chunks: []Chunk{
			{ID: 10, CompanyID: 3, Content: "printer offline"},
			{ID: 11, CompanyID: 3, Content: "network queue"},
		},
	}
	vectorStore := &fakeVectorStore{}
	indexer := NewIndexer(source, fakeEmbedder{dim: 3}, vectorStore, &fakeMarker{})

	result, err := indexer.Run(context.Background(), Options{
		CompanyID:        3,
		CollectionPrefix: "kb_chunks",
		BatchSize:        10,
		DryRun:           true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Points != 0 || len(vectorStore.points) != 0 {
		t.Fatalf("dry-run wrote vectors: points=%d upserts=%d", result.Points, len(vectorStore.points))
	}
	wantRunes := len([]rune("printer offline")) + len([]rune("network queue"))
	if result.EstimatedInputRunes != wantRunes {
		t.Fatalf("EstimatedInputRunes = %d, want %d", result.EstimatedInputRunes, wantRunes)
	}
}

func TestIndexerRejectsCrossTenantSourceLeak(t *testing.T) {
	source := &leakySource{
		chunks: []Chunk{
			{ID: 1, CompanyID: 1, Content: "co1"},
			{ID: 2, CompanyID: 2, Content: "co2 leaked despite filter"},
		},
	}
	vectorStore := &fakeVectorStore{}
	indexer := NewIndexer(source, fakeEmbedder{dim: 2}, vectorStore, &fakeMarker{})

	_, err := indexer.Run(context.Background(), Options{
		CompanyID:        1,
		CollectionPrefix: "kb_chunks",
		BatchSize:        10,
	})
	if err == nil {
		t.Fatal("Run() error = nil, want cross-tenant rejection")
	}
	if len(vectorStore.points) != 0 {
		t.Fatalf("points upserted = %d, want 0 (must reject before write)", len(vectorStore.points))
	}
}

func TestIndexerMarksEmbeddingRefsWithCompanyContext(t *testing.T) {
	source := &fakeChunkSource{
		chunks: []Chunk{
			{ID: 10, ArticleID: 1, CompanyID: 3, SourceReportID: 100, ChunkIndex: 0, Content: "printer offline"},
		},
	}
	marker := &fakeMarker{}
	indexer := NewIndexer(source, fakeEmbedder{dim: 3}, &fakeVectorStore{}, marker)

	_, err := indexer.Run(context.Background(), Options{
		CompanyID:        3,
		CollectionPrefix: "kb_chunks",
		BatchSize:        10,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(marker.companyIDs) != 1 || marker.companyIDs[0] != 3 {
		t.Fatalf("marker company IDs = %#v, want [3]", marker.companyIDs)
	}
}

func TestIndexerDryRunCountsWithoutEmbedding(t *testing.T) {
	source := &fakeChunkSource{
		chunks: []Chunk{
			{ID: 1, CompanyID: 1, Content: "a"},
			{ID: 2, CompanyID: 1, Content: "b"},
		},
	}
	vectorStore := &fakeVectorStore{}
	marker := &fakeMarker{}
	indexer := NewIndexer(source, &countingEmbedder{}, vectorStore, marker)

	result, err := indexer.Run(context.Background(), Options{
		CompanyID:        1,
		CollectionPrefix: "kb_chunks",
		BatchSize:        10,
		DryRun:           true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Chunks != 2 {
		t.Fatalf("Chunks = %d, want 2", result.Chunks)
	}
	if result.Points != 0 {
		t.Fatalf("Points = %d, want 0 (dry run must not upsert)", result.Points)
	}
	if len(vectorStore.points) != 0 {
		t.Fatalf("vectorStore.points = %d, want 0", len(vectorStore.points))
	}
	if marker.refs != nil {
		t.Fatalf("marker called in dry-run mode")
	}
}

type leakySource struct {
	chunks []Chunk
}

func (s *leakySource) Next(_ context.Context, _ int64, afterID int64, limit int) ([]Chunk, error) {
	out := make([]Chunk, 0, limit)
	for _, chunk := range s.chunks {
		if chunk.ID <= afterID {
			continue
		}
		out = append(out, chunk)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

type countingEmbedder struct {
	calls int
}

func (e *countingEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	e.calls++
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{1, 2}
	}
	return out, nil
}

func (e *countingEmbedder) Dim() int { return 2 }

func TestIndexerUsesBatchSize(t *testing.T) {
	source := &fakeChunkSource{
		chunks: []Chunk{
			{ID: 1, CompanyID: 1, Content: "a"},
			{ID: 2, CompanyID: 1, Content: "b"},
			{ID: 3, CompanyID: 1, Content: "c"},
		},
	}
	indexer := NewIndexer(source, fakeEmbedder{dim: 2}, &fakeVectorStore{}, &fakeMarker{})

	result, err := indexer.Run(context.Background(), Options{
		CollectionPrefix: "kb_chunks",
		BatchSize:        2,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Batches != 2 {
		t.Fatalf("Batches = %d, want 2", result.Batches)
	}
	if source.calls != 3 {
		t.Fatalf("source calls = %d, want 3", source.calls)
	}
}

type fakeChunkSource struct {
	chunks []Chunk
	calls  int
}

func (s *fakeChunkSource) Next(ctx context.Context, companyID int64, afterID int64, limit int) ([]Chunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.calls++
	out := make([]Chunk, 0, limit)
	for _, chunk := range s.chunks {
		if chunk.ID <= afterID {
			continue
		}
		if companyID > 0 && chunk.CompanyID != companyID {
			continue
		}
		out = append(out, chunk)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

type fakeEmbedder struct {
	dim int
}

func (e fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	vectors := make([][]float32, 0, len(texts))
	for range texts {
		vector := make([]float32, e.dim)
		for i := range vector {
			vector[i] = float32(i + 1)
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func (e fakeEmbedder) Dim() int {
	return e.dim
}

type fakeVectorStore struct {
	collection  string
	collections []string
	dim         int
	points      []ports.VectorPoint
}

func (s *fakeVectorStore) EnsureCollection(ctx context.Context, name string, dim int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.collection = name
	s.dim = dim
	return nil
}

func (s *fakeVectorStore) Upsert(ctx context.Context, collection string, points []ports.VectorPoint) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.collection = collection
	s.collections = append(s.collections, collection)
	s.points = append(s.points, points...)
	return nil
}

func (s *fakeVectorStore) Search(ctx context.Context, q ports.VectorSearch) ([]ports.VectorHit, error) {
	return nil, nil
}

func (s *fakeVectorStore) Delete(ctx context.Context, collection string, ids []string) error {
	return nil
}

type fakeMarker struct {
	refs       map[int64]string
	companyIDs []int64
}

func (m *fakeMarker) Mark(ctx context.Context, refs []ChunkRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if companyID, ok := ctxkey.CompanyID(ctx); ok {
		m.companyIDs = append(m.companyIDs, companyID)
	}
	if m.refs == nil {
		m.refs = make(map[int64]string)
	}
	for _, ref := range refs {
		m.refs[ref.ChunkID] = ref.VectorRef
	}
	return nil
}
