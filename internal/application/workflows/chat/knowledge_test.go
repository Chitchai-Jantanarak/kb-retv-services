package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/my/app/internal/ai/rag"
)

type knowledgeFTS struct {
	chunks []rag.FTSChunk
	err    error
	limit  int
}

func (s *knowledgeFTS) SearchChunks(_ context.Context, _ []int64, _ string, limit int) ([]rag.FTSChunk, error) {
	s.limit = limit
	if s.err != nil {
		return nil, s.err
	}
	return s.chunks, nil
}

type stubEmbedder struct {
	vectors map[string][]float32
	err     error
	calls   int
}

func (s *stubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v, ok := s.vectors[t]
		if !ok {
			v = []float32{0, 0}
		}
		out[i] = v
	}
	return out, nil
}

func TestKnowledgeContextRerankOrdersByCosineNotFTRelevance(t *testing.T) {
	fts := &knowledgeFTS{chunks: []rag.FTSChunk{
		{ChunkID: 1, Title: "Off Topic", Content: "off topic content", Relevance: 20.0},
		{ChunkID: 2, Title: "On Topic", Content: "on topic content", Relevance: 5.0},
	}}
	emb := &stubEmbedder{vectors: map[string][]float32{
		"query":              {1, 0},
		"off topic content":  {0, 1},
		"on topic content":   {1, 0},
	}}

	w := &Workflow{fts: fts, knowledgeReranker: emb}
	sources, _ := w.knowledgeContext(context.Background(), 1, "query")

	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1 (off-topic below knowledgeMinSimilarity)", len(sources))
	}
	if sources[0].ID != "chunk:2" {
		t.Fatalf("sources[0].ID = %q, want chunk:2 (cosine winner), got sources=%+v", sources[0].ID, sources)
	}
	if fts.limit != knowledgeCandidateLimit {
		t.Fatalf("fts.limit = %d, want %d", fts.limit, knowledgeCandidateLimit)
	}
	if emb.calls != 1 {
		t.Fatalf("embed calls = %d, want 1 (single batch)", emb.calls)
	}
}

func TestKnowledgeContextRerankDropsBelowMinSimilarity(t *testing.T) {
	fts := &knowledgeFTS{chunks: []rag.FTSChunk{
		{ChunkID: 1, Title: "Unrelated", Content: "unrelated content", Relevance: 10.0},
	}}
	emb := &stubEmbedder{vectors: map[string][]float32{
		"query":             {1, 0},
		"unrelated content": {0, 1},
	}}

	w := &Workflow{fts: fts, knowledgeReranker: emb}
	sources, block := w.knowledgeContext(context.Background(), 1, "query")

	if len(sources) != 0 {
		t.Fatalf("sources = %+v, want none below knowledgeMinSimilarity", sources)
	}
	if block != "" {
		t.Fatalf("block = %q, want empty", block)
	}
}

func TestKnowledgeContextRerankCapsAtChunkLimit(t *testing.T) {
	vectors := map[string][]float32{"query": {1, 0}}
	chunks := make([]rag.FTSChunk, 0, knowledgeChunkLimit+2)
	for i := 0; i < knowledgeChunkLimit+2; i++ {
		content := "content"
		for j := 0; j <= i; j++ {
			content += "x"
		}
		chunks = append(chunks, rag.FTSChunk{ChunkID: int64(i + 1), Title: "T", Content: content, Relevance: float64(i)})
		vectors[content] = []float32{1, 0}
	}
	fts := &knowledgeFTS{chunks: chunks}
	emb := &stubEmbedder{vectors: vectors}

	w := &Workflow{fts: fts, knowledgeReranker: emb}
	sources, _ := w.knowledgeContext(context.Background(), 1, "query")

	if len(sources) != knowledgeChunkLimit {
		t.Fatalf("len(sources) = %d, want %d", len(sources), knowledgeChunkLimit)
	}
}

func TestKnowledgeContextNoRerankerUsesRelevanceGateUnchanged(t *testing.T) {
	fts := &knowledgeFTS{chunks: []rag.FTSChunk{
		{ChunkID: 1, Title: "A", Content: "alpha", Relevance: 10.0},
		{ChunkID: 2, Title: "B", Content: "beta", Relevance: 1.0},
	}}
	w := &Workflow{fts: fts}
	sources, _ := w.knowledgeContext(context.Background(), 1, "query")

	if len(sources) != 1 || sources[0].ID != "chunk:1" {
		t.Fatalf("sources = %+v, want only chunk:1 via gateChunks", sources)
	}
	if fts.limit != knowledgeChunkLimit {
		t.Fatalf("fts.limit = %d, want %d", fts.limit, knowledgeChunkLimit)
	}
}

func TestKnowledgeContextEmbedFailureFallsBackToRelevancePath(t *testing.T) {
	fts := &knowledgeFTS{chunks: []rag.FTSChunk{
		{ChunkID: 1, Title: "A", Content: "alpha", Relevance: 10.0},
	}}
	emb := &stubEmbedder{err: errors.New("embed down")}
	w := &Workflow{fts: fts, knowledgeReranker: emb}

	sources, _ := w.knowledgeContext(context.Background(), 1, "query")

	if len(sources) != 1 || sources[0].ID != "chunk:1" {
		t.Fatalf("sources = %+v, want fallback to relevance path with chunk:1", sources)
	}
}
