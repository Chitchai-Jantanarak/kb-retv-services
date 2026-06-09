package rag

import (
	"context"
	"testing"

	"github.com/my/app/internal/domain/kb"
)

func TestContextAssemblerExpandsChildChunksToParentsAndDedupes(t *testing.T) {
	source := &stubParentChunkSource{
		contexts: []kb.ChunkContext{
			{
				ChildID:   "11",
				ParentID:  "99",
				ArticleID: "7",
				CompanyID: 3,
				Title:     "Reset printer tray",
				Content:   "Parent context with all reset steps.",
			},
			{
				ChildID:   "12",
				ParentID:  "99",
				ArticleID: "7",
				CompanyID: 3,
				Title:     "Reset printer tray",
				Content:   "Parent context with all reset steps.",
			},
		},
	}
	assembler := NewContextAssembler(source)

	out, err := assembler.Assemble(context.Background(), Query{CompanyID: 3}, []Candidate{
		{ID: "7:11", ArticleID: "7", ChunkID: "11", Title: "Child A", Content: "small A", Source: "vector", Score: 0.9},
		{ID: "7:12", ArticleID: "7", ChunkID: "12", Title: "Child B", Content: "small B", Source: "vector", Score: 0.8},
		{ID: "graph-1", Title: "Graph", Content: "graph content", Source: "graph", Score: 0.7},
	})
	if err != nil {
		t.Fatalf("Assemble() error = %v", err)
	}

	if len(source.query.IDs) != 2 || source.query.IDs[0] != "11" || source.query.IDs[1] != "12" {
		t.Fatalf("parent query IDs = %#v, want child IDs 11,12", source.query.IDs)
	}
	if len(out) != 2 {
		t.Fatalf("assembled candidates = %d, want parent plus graph", len(out))
	}
	if out[0].ID != "7:99" {
		t.Fatalf("first ID = %q, want parent candidate 7:99", out[0].ID)
	}
	if out[0].ParentChunkID != "99" || out[0].ChunkID != "11" {
		t.Fatalf("chunk metadata = %+v, want child 11 parent 99", out[0])
	}
	if out[0].Content != "Parent context with all reset steps." {
		t.Fatalf("content = %q, want parent context", out[0].Content)
	}
	if out[1].ID != "graph-1" {
		t.Fatalf("second ID = %q, want non-chunk candidate preserved", out[1].ID)
	}
}

type stubParentChunkSource struct {
	query    kb.ChunkQuery
	contexts []kb.ChunkContext
	err      error
}

func (s *stubParentChunkSource) ParentChunksByChildIDs(ctx context.Context, query kb.ChunkQuery) ([]kb.ChunkContext, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.query = query
	if s.err != nil {
		return nil, s.err
	}
	return s.contexts, nil
}
