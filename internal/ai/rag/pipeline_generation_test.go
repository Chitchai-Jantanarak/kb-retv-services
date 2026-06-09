package rag

import (
	"context"
	"errors"
	"testing"

	"github.com/my/app/internal/domain/kb"
)

type stubGenerator struct {
	draft      string
	err        error
	calls      int
	candidates []Candidate
}

func (g *stubGenerator) Generate(_ context.Context, _ Query, candidates []Candidate) (string, error) {
	g.calls++
	g.candidates = append([]Candidate(nil), candidates...)
	return g.draft, g.err
}

type stubCritic struct {
	res   CritiqueResult
	err   error
	calls int
}

func (c *stubCritic) Critique(_ context.Context, _ Query, _ string, _ []Candidate) (CritiqueResult, error) {
	c.calls++
	return c.res, c.err
}

func TestPipelineUsesGeneratorWhenConfigured(t *testing.T) {
	gen := &stubGenerator{draft: "draft answer"}
	pipeline := NewPipeline(Config{
		Extractor:           DefaultExtractor{},
		Retrievers:          []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "x"}}}},
		Reranker:            LexicalReranker{},
		Compressor:          Compressor{MaxRunes: 100},
		CRAG:                fakeCRAG{verdict: VerdictRelevant},
		Generator:           gen,
		ConfidenceThreshold: 0.85,
		Workers:             1,
	})
	res, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "help"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gen.calls != 1 {
		t.Fatalf("generator calls = %d, want 1", gen.calls)
	}
	if res.Draft != "draft answer" {
		t.Fatalf("Draft = %q", res.Draft)
	}
}

func TestPipelineAssemblesParentContextBeforeGeneration(t *testing.T) {
	gen := &stubGenerator{draft: "draft answer"}
	pipeline := NewPipeline(Config{
		Extractor: DefaultExtractor{},
		Retrievers: []Retriever{&recordingRetriever{candidates: []Candidate{
			{ID: "7:11", ArticleID: "7", ChunkID: "11", Title: "Child", Content: "child snippet", Source: "vector", Score: 0.9},
		}}},
		ContextAssembler: NewContextAssembler(&stubParentChunkSource{
			contexts: []kb.ChunkContext{{
				ChildID:   "11",
				ParentID:  "99",
				ArticleID: "7",
				CompanyID: 1,
				Title:     "Parent",
				Content:   "parent context for generation",
			}},
		}),
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 100},
		CRAG:       fakeCRAG{verdict: VerdictRelevant},
		Generator:  gen,
		Workers:    1,
	})

	res, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "help"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(gen.candidates) != 1 || gen.candidates[0].Content != "parent context for generation" {
		t.Fatalf("generator candidates = %+v, want parent context", gen.candidates)
	}
	if res.Context != "Parent\nparent context for generation" {
		t.Fatalf("Context = %q, want parent context", res.Context)
	}
}

func TestPipelineSkipsGeneratorOnIrrelevantVerdict(t *testing.T) {
	gen := &stubGenerator{draft: "should not appear"}
	pipeline := NewPipeline(Config{
		Extractor:  DefaultExtractor{},
		Retrievers: []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "x"}}}},
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 100},
		CRAG:       fakeCRAG{verdict: VerdictIrrelevant},
		Generator:  gen,
		Workers:    1,
	})
	res, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "help"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gen.calls != 0 {
		t.Fatalf("generator calls = %d, want 0 on irrelevant", gen.calls)
	}
	if res.Draft != "" {
		t.Fatalf("Draft = %q, want empty on irrelevant", res.Draft)
	}
}

func TestPipelineKeepsDraftWhenGeneratorErrors(t *testing.T) {
	gen := &stubGenerator{err: errors.New("provider 500")}
	pipeline := NewPipeline(Config{
		Extractor:  DefaultExtractor{},
		Retrievers: []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "x"}}}},
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 100},
		CRAG:       fakeCRAG{verdict: VerdictRelevant},
		Generator:  gen,
		Workers:    1,
	})
	res, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "help"})
	if err != nil {
		t.Fatalf("Run err = %v, want nil (generator failures must be swallowed)", err)
	}
	if res.Draft != "" {
		t.Fatalf("Draft = %q, want empty after generator error", res.Draft)
	}
}

func TestPipelineCriticDowngradesAutoToDraftWhenSendFalse(t *testing.T) {
	pipeline := NewPipeline(Config{
		Extractor:           DefaultExtractor{},
		Retrievers:          []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "x"}}}},
		Reranker:            LexicalReranker{},
		Compressor:          Compressor{MaxRunes: 100},
		CRAG:                fakeCRAG{verdict: VerdictRelevant},
		Generator:           &stubGenerator{draft: "draft"},
		Critic:              &stubCritic{res: CritiqueResult{Supported: false, Send: false}},
		ConfidenceThreshold: 0.85,
		Workers:             1,
	})
	res, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "help"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Decision != DecisionDraft {
		t.Fatalf("Decision = %q, want draft (critic refused send)", res.Decision)
	}
	if res.Critique.Send {
		t.Fatalf("Critique.Send = true, want false")
	}
}

func TestPipelineSkipsCriticWhenDecisionIsDraft(t *testing.T) {
	critic := &stubCritic{res: CritiqueResult{Supported: false, Send: false}}
	pipeline := NewPipeline(Config{
		Extractor:           DefaultExtractor{},
		Retrievers:          []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "x"}}}},
		Reranker:            LexicalReranker{},
		Compressor:          Compressor{MaxRunes: 100},
		CRAG:                fakeCRAG{result: CRAGResult{Verdict: VerdictPartial, Confidence: 0.7}},
		Generator:           &stubGenerator{draft: "draft"},
		Critic:              critic,
		ConfidenceThreshold: 0.85,
		Workers:             1,
	})

	res, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "help"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if critic.calls != 0 {
		t.Fatalf("critic calls = %d, want 0 for draft decision", critic.calls)
	}
	if res.Decision != DecisionDraft {
		t.Fatalf("Decision = %q, want draft", res.Decision)
	}
}

func TestPipelineCriticKeepsAutoWhenSendTrue(t *testing.T) {
	pipeline := NewPipeline(Config{
		Extractor:           DefaultExtractor{},
		Retrievers:          []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "x"}}}},
		Reranker:            LexicalReranker{},
		Compressor:          Compressor{MaxRunes: 100},
		CRAG:                fakeCRAG{verdict: VerdictRelevant},
		Generator:           &stubGenerator{draft: "draft"},
		Critic:              &stubCritic{res: CritiqueResult{Supported: true, Send: true}},
		ConfidenceThreshold: 0.85,
		Workers:             1,
	})
	res, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "help"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Decision != DecisionAuto {
		t.Fatalf("Decision = %q, want auto", res.Decision)
	}
}

func TestPipelineCriticErrorIsSwallowed(t *testing.T) {
	pipeline := NewPipeline(Config{
		Extractor:           DefaultExtractor{},
		Retrievers:          []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "x"}}}},
		Reranker:            LexicalReranker{},
		Compressor:          Compressor{MaxRunes: 100},
		CRAG:                fakeCRAG{verdict: VerdictRelevant},
		Generator:           &stubGenerator{draft: "draft"},
		Critic:              &stubCritic{err: errors.New("critic 500")},
		ConfidenceThreshold: 0.85,
		Workers:             1,
	})
	res, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "help"})
	if err != nil {
		t.Fatalf("Run err = %v, want nil", err)
	}
	if res.Decision != DecisionAuto {
		t.Fatalf("Decision = %q, want auto (critic error should not change decision)", res.Decision)
	}
}
