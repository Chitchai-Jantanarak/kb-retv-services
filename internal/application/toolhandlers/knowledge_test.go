package toolhandlers

import (
	"context"
	"testing"

	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/application/tools"
)

type stubFTS struct {
	chunks []rag.FTSChunk
	query  string
	limit  int
}

func (s *stubFTS) SearchChunks(_ context.Context, _ []int64, query string, limit int) ([]rag.FTSChunk, error) {
	s.query = query
	s.limit = limit
	return s.chunks, nil
}

func TestKnowledgeHandlerMapsChunksToRows(t *testing.T) {
	t.Parallel()
	fts := &stubFTS{chunks: []rag.FTSChunk{{ArticleID: 4, Title: "Motor fix", Content: "replace the belt and reboot"}}}
	h := NewKnowledge(fts)

	rows, err := h.Run(context.Background(), skeleton.Query{
		Text:     "belt",
		Coverage: []int64{3},
		Tool:     tools.Tool{Limit: 5},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0]["article"] != "Motor fix" || rows[0]["article_id"] != "4" {
		t.Fatalf("row = %v, want article=Motor fix article_id=4", rows[0])
	}
	if fts.query != "belt" || fts.limit != 5 {
		t.Fatalf("fts called with query=%q limit=%d, want belt/5", fts.query, fts.limit)
	}
}

func TestKnowledgeHandlerDefaultsLimit(t *testing.T) {
	t.Parallel()
	fts := &stubFTS{}
	h := NewKnowledge(fts)
	if _, err := h.Run(context.Background(), skeleton.Query{Text: "x", Tool: tools.Tool{Limit: 0}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fts.limit != 5 {
		t.Fatalf("limit = %d, want default 5", fts.limit)
	}
}
