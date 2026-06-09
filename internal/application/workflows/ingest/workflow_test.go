package ingest

import (
	"context"
	"errors"
	"testing"

	"github.com/my/app/internal/application/services/kbbootstrap"
	"github.com/my/app/internal/application/services/vectorindex"
	"github.com/my/app/internal/domain/ports"
)

func TestWorkflowRejectsZeroCompanyID(t *testing.T) {
	wf := mustWorkflow(t, &countingBootstrapSource{}, &countingArticleStore{}, &countingChunkSource{})

	_, err := wf.Run(context.Background(), 0, Options{})
	if err == nil {
		t.Fatal("Run(companyID=0) error = nil, want positive companyID error")
	}
}

func TestWorkflowFailureCases(t *testing.T) {
	cases := []struct {
		name           string
		sourceErr      error
		storeErr       error
		chunkErr       error
		wantErrSubstr  string
		wantStoreCalls int
		wantIndexCalls int
	}{
		{
			name:           "bootstrap_source_failure_aborts_indexer",
			sourceErr:      errors.New("reports query failed"),
			wantErrSubstr:  "bootstrap",
			wantStoreCalls: 0,
			wantIndexCalls: 0,
		},
		{
			name:           "article_store_failure_aborts_indexer",
			storeErr:       errors.New("insert failed"),
			wantErrSubstr:  "bootstrap",
			wantStoreCalls: 1,
			wantIndexCalls: 0,
		},
		{
			name:           "indexer_failure_propagates_after_bootstrap",
			chunkErr:       errors.New("chunk fetch failed"),
			wantErrSubstr:  "index",
			wantStoreCalls: 1,
			wantIndexCalls: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := &countingBootstrapSource{
				reports: []kbbootstrap.ReportRecord{
					{ID: 1, CompanyID: 7, Code: "R-1", Title: "Boot"},
				},
				err: tc.sourceErr,
			}
			store := &countingArticleStore{err: tc.storeErr}
			chunks := &countingChunkSource{err: tc.chunkErr}
			wf := mustWorkflow(t, source, store, chunks)

			_, err := wf.Run(context.Background(), 7, Options{})
			if err == nil {
				t.Fatalf("Run() error = nil, want %s failure", tc.wantErrSubstr)
			}
			if !containsSubstr(err.Error(), tc.wantErrSubstr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErrSubstr)
			}
			if store.calls != tc.wantStoreCalls {
				t.Fatalf("store.calls = %d, want %d", store.calls, tc.wantStoreCalls)
			}
			if chunks.calls != tc.wantIndexCalls {
				t.Fatalf("chunkSource.calls = %d, want %d", chunks.calls, tc.wantIndexCalls)
			}
		})
	}
}

func TestWorkflowEmptyCompanySucceeds(t *testing.T) {
	source := &countingBootstrapSource{reports: nil}
	store := &countingArticleStore{}
	chunks := &countingChunkSource{}
	wf := mustWorkflow(t, source, store, chunks)

	result, err := wf.Run(context.Background(), 7, Options{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Reports != 0 || result.Articles != 0 || result.IndexedChunks != 0 {
		t.Fatalf("non-zero result for empty company: %+v", result)
	}
}

func TestWorkflowDryRunSkipsEmbedding(t *testing.T) {
	source := &countingBootstrapSource{
		reports: []kbbootstrap.ReportRecord{{ID: 1, CompanyID: 7, Code: "R-1", Title: "Boot", ProblemFull: "x"}},
	}
	store := &countingArticleStore{}
	chunks := &countingChunkSource{
		chunks: []vectorindex.Chunk{{ID: 100, CompanyID: 7, Content: "x"}},
	}
	embedder := &countingEmbedder{}
	wf := mustWorkflowWithEmbedder(t, source, store, chunks, embedder)

	result, err := wf.Run(context.Background(), 7, Options{DryRun: true})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if embedder.calls != 0 {
		t.Fatalf("embedder.calls = %d, want 0 (dry-run must not embed)", embedder.calls)
	}
	if result.IndexedPoints != 0 {
		t.Fatalf("IndexedPoints = %d, want 0", result.IndexedPoints)
	}
	if result.IndexedChunks == 0 {
		t.Fatal("IndexedChunks = 0, want > 0 (dry-run still counts)")
	}
}

func TestWorkflowContextCancellationPropagates(t *testing.T) {
	source := &countingBootstrapSource{
		reports: []kbbootstrap.ReportRecord{{ID: 1, CompanyID: 7, Title: "Boot"}},
	}
	store := &countingArticleStore{}
	chunks := &countingChunkSource{}
	wf := mustWorkflow(t, source, store, chunks)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := wf.Run(ctx, 7, Options{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if source.calls != 0 {
		t.Fatalf("source.calls = %d, want 0 (cancelled ctx must short-circuit)", source.calls)
	}
}

func mustWorkflow(t *testing.T, src *countingBootstrapSource, store *countingArticleStore, chunks *countingChunkSource) *Workflow {
	t.Helper()
	return mustWorkflowWithEmbedder(t, src, store, chunks, &countingEmbedder{})
}

func mustWorkflowWithEmbedder(t *testing.T, src *countingBootstrapSource, store *countingArticleStore, chunks *countingChunkSource, embedder *countingEmbedder) *Workflow {
	t.Helper()
	boot := kbbootstrap.NewBootstrapper(src, store)
	idx := vectorindex.NewIndexer(chunks, embedder, &nopVectorStore{}, &nopMarker{})
	wf, err := New(Config{
		Bootstrapper:     boot,
		Indexer:          idx,
		CollectionPrefix: "kb_chunks",
		Model:            "hash-dev",
		ChunkRunes:       40,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return wf
}

type countingBootstrapSource struct {
	reports []kbbootstrap.ReportRecord
	err     error
	calls   int
}

func (s *countingBootstrapSource) Reports(_ context.Context, companyID int64, _ int) ([]kbbootstrap.ReportRecord, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	out := make([]kbbootstrap.ReportRecord, 0, len(s.reports))
	for _, r := range s.reports {
		if companyID > 0 && r.CompanyID != companyID {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

type countingArticleStore struct {
	err   error
	calls int
}

func (s *countingArticleStore) InsertArticle(_ context.Context, _ kbbootstrap.ArticleRecord, _ []string) (bool, error) {
	s.calls++
	if s.err != nil {
		return false, s.err
	}
	return true, nil
}

type countingChunkSource struct {
	chunks []vectorindex.Chunk
	err    error
	calls  int
}

func (s *countingChunkSource) Next(_ context.Context, _ int64, afterID int64, limit int) ([]vectorindex.Chunk, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	out := make([]vectorindex.Chunk, 0, limit)
	for _, c := range s.chunks {
		if c.ID <= afterID {
			continue
		}
		out = append(out, c)
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

type nopVectorStore struct{}

func (nopVectorStore) EnsureCollection(_ context.Context, _ string, _ int) error { return nil }
func (nopVectorStore) Upsert(_ context.Context, _ string, _ []ports.VectorPoint) error {
	return nil
}

func (nopVectorStore) Search(_ context.Context, _ ports.VectorSearch) ([]ports.VectorHit, error) {
	return nil, nil
}
func (nopVectorStore) Delete(_ context.Context, _ string, _ []string) error { return nil }

type nopMarker struct{}

func (nopMarker) Mark(_ context.Context, _ []vectorindex.ChunkRef) error { return nil }

func containsSubstr(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
