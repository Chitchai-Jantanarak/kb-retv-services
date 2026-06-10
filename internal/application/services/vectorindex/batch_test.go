package vectorindex

import (
	"context"
	"slices"
	"testing"
)

type fakeMetaSource struct {
	chunks     map[int64]Chunk
	companyIDs []int64
}

func (s *fakeMetaSource) MetaByIDs(ctx context.Context, companyID int64, ids []int64) ([]Chunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.companyIDs = append(s.companyIDs, companyID)
	out := make([]Chunk, 0, len(ids))
	for _, id := range ids {
		if c, ok := s.chunks[id]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

func TestParseBatchKey(t *testing.T) {
	company, chunk, err := ParseBatchKey("2:10")
	if err != nil {
		t.Fatalf("ParseBatchKey error = %v", err)
	}
	if company != 2 || chunk != 10 {
		t.Fatalf("got company=%d chunk=%d, want 2/10", company, chunk)
	}
}

func TestParseBatchKeyRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"10", "2:", ":10", "a:b", "2:10:3", ""} {
		if _, _, err := ParseBatchKey(bad); err == nil {
			t.Fatalf("ParseBatchKey(%q) error = nil, want error", bad)
		}
	}
}

func TestFormatBatchKeyRoundTrips(t *testing.T) {
	company, chunk, err := ParseBatchKey(FormatBatchKey(7, 42))
	if err != nil || company != 7 || chunk != 42 {
		t.Fatalf("round trip got company=%d chunk=%d err=%v", company, chunk, err)
	}
}

func TestBatchIngestorUpsertsAndMarksPerCompany(t *testing.T) {
	meta := &fakeMetaSource{chunks: map[int64]Chunk{
		10: {ID: 10, ArticleID: 1, CompanyID: 2, SourceReportID: 100, ChunkIndex: 0},
		20: {ID: 20, ArticleID: 5, CompanyID: 3, SourceReportID: 200, ChunkIndex: 1},
	}}
	vs := &fakeVectorStore{}
	mk := &fakeMarker{}
	ing := NewBatchIngestor(meta, vs, mk)

	res, err := ing.Ingest(context.Background(), Options{CollectionPrefix: "kb_chunks", Model: "gemini-embedding-2"}, []KeyedVector{
		{Key: "2:10", Vector: []float32{0.1, 0.2, 0.3}},
		{Key: "3:20", Vector: []float32{0.4, 0.5, 0.6}},
	})
	if err != nil {
		t.Fatalf("Ingest error = %v", err)
	}
	if res.Points != 2 {
		t.Fatalf("Points = %d, want 2", res.Points)
	}
	if len(vs.points) != 2 {
		t.Fatalf("upserted points = %d, want 2", len(vs.points))
	}
	if !slices.Contains(vs.collections, "kb_chunks__2") || !slices.Contains(vs.collections, "kb_chunks__3") {
		t.Fatalf("collections = %v, want kb_chunks__2 and kb_chunks__3", vs.collections)
	}
	if mk.refs[10] != "10" || mk.refs[20] != "20" {
		t.Fatalf("marker refs = %v", mk.refs)
	}
	if !slices.Contains(mk.companyIDs, 2) || !slices.Contains(mk.companyIDs, 3) {
		t.Fatalf("marker company IDs = %v, want 2 and 3", mk.companyIDs)
	}
	if !slices.Contains(meta.companyIDs, 2) || !slices.Contains(meta.companyIDs, 3) {
		t.Fatalf("meta lookups = %v, want routed per company 2 and 3", meta.companyIDs)
	}
}

func TestBatchIngestorRejectsEmptyVector(t *testing.T) {
	meta := &fakeMetaSource{chunks: map[int64]Chunk{10: {ID: 10, CompanyID: 2}}}
	vs := &fakeVectorStore{}
	ing := NewBatchIngestor(meta, vs, &fakeMarker{})

	_, err := ing.Ingest(context.Background(), Options{CollectionPrefix: "kb_chunks"}, []KeyedVector{
		{Key: "2:10", Vector: nil},
	})
	if err == nil {
		t.Fatal("Ingest error = nil, want error for empty vector")
	}
	if len(vs.points) != 0 {
		t.Fatalf("points upserted = %d, want 0 (reject before write)", len(vs.points))
	}
}

func TestBatchIngestorRejectsDimMismatch(t *testing.T) {
	meta := &fakeMetaSource{chunks: map[int64]Chunk{
		10: {ID: 10, CompanyID: 2},
		11: {ID: 11, CompanyID: 2},
	}}
	vs := &fakeVectorStore{}
	ing := NewBatchIngestor(meta, vs, &fakeMarker{})

	_, err := ing.Ingest(context.Background(), Options{CollectionPrefix: "kb_chunks"}, []KeyedVector{
		{Key: "2:10", Vector: []float32{0.1, 0.2, 0.3}},
		{Key: "2:11", Vector: []float32{0.4, 0.5}},
	})
	if err == nil {
		t.Fatal("Ingest error = nil, want error for inconsistent vector dim")
	}
	if len(vs.points) != 0 {
		t.Fatalf("points upserted = %d, want 0 (reject before write)", len(vs.points))
	}
}

func TestBatchIngestorRejectsUnknownChunk(t *testing.T) {
	meta := &fakeMetaSource{chunks: map[int64]Chunk{10: {ID: 10, CompanyID: 2}}}
	vs := &fakeVectorStore{}
	ing := NewBatchIngestor(meta, vs, &fakeMarker{})

	_, err := ing.Ingest(context.Background(), Options{CollectionPrefix: "kb_chunks"}, []KeyedVector{
		{Key: "2:99", Vector: []float32{0.1}},
	})
	if err == nil {
		t.Fatal("Ingest error = nil, want error for result of unknown chunk")
	}
	if len(vs.points) != 0 {
		t.Fatalf("points upserted = %d, want 0", len(vs.points))
	}
}

func TestBatchIngestorRejectsCompanyMismatch(t *testing.T) {
	meta := &fakeMetaSource{chunks: map[int64]Chunk{10: {ID: 10, CompanyID: 3}}}
	vs := &fakeVectorStore{}
	ing := NewBatchIngestor(meta, vs, &fakeMarker{})

	_, err := ing.Ingest(context.Background(), Options{CollectionPrefix: "kb_chunks"}, []KeyedVector{
		{Key: "2:10", Vector: []float32{0.1}},
	})
	if err == nil {
		t.Fatal("Ingest error = nil, want error when key company != chunk company")
	}
	if len(vs.points) != 0 {
		t.Fatalf("points upserted = %d, want 0 (reject before write)", len(vs.points))
	}
}
