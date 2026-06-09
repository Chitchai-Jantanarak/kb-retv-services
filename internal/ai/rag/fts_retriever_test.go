package rag

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubFTSSource struct {
	chunks []FTSChunk
	err    error
	calls  int
	lastQ  string
	lastCo int64
}

func (s *stubFTSSource) SearchChunks(_ context.Context, companyID int64, query string, _ int) ([]FTSChunk, error) {
	s.calls++
	s.lastQ = query
	s.lastCo = companyID
	if s.err != nil {
		return nil, s.err
	}
	return s.chunks, nil
}

func TestNewFTSRetrieverRejectsNilSource(t *testing.T) {
	_, err := NewFTSRetriever(nil)
	if err == nil || !strings.Contains(err.Error(), "source is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestFTSRetrieverEmptyQuery(t *testing.T) {
	src := &stubFTSSource{}
	r, err := NewFTSRetriever(src)
	if err != nil {
		t.Fatalf("NewFTSRetriever: %v", err)
	}
	out, err := r.Retrieve(context.Background(), Query{CompanyID: 1, Text: "   "}, Meta{})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(out) != 0 || src.calls != 0 {
		t.Fatalf("expected no retrieval; calls=%d out=%d", src.calls, len(out))
	}
}

func TestFTSRetrieverPropagatesSourceError(t *testing.T) {
	r, err := NewFTSRetriever(&stubFTSSource{err: errors.New("db down")})
	if err != nil {
		t.Fatalf("NewFTSRetriever: %v", err)
	}
	_, err = r.Retrieve(context.Background(), Query{CompanyID: 1, Text: "x"}, Meta{})
	if err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("err = %v", err)
	}
}

func TestFTSRetrieverMapsRowsToCandidates(t *testing.T) {
	src := &stubFTSSource{chunks: []FTSChunk{
		{ChunkID: 1, ArticleID: 10, Title: "Printer", Content: "restart", Relevance: 0.9},
		{ChunkID: 2, ArticleID: 11, Title: "Network", Content: "check cable", Relevance: 0.5},
	}}
	r, err := NewFTSRetriever(src)
	if err != nil {
		t.Fatalf("NewFTSRetriever: %v", err)
	}
	out, err := r.Retrieve(context.Background(), Query{CompanyID: 7, Text: "printer", Limit: 2}, Meta{})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if out[0].ID != "chunk:1" || out[0].Source != "fts" || out[0].Score != 0.9 {
		t.Fatalf("candidate[0] = %+v", out[0])
	}
	if src.lastCo != 7 || src.lastQ != "printer" {
		t.Fatalf("source got company=%d q=%q", src.lastCo, src.lastQ)
	}
}
