package rag

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/my/app/internal/domain/kb"
	"github.com/my/app/internal/infra/llm/llmerr"
)

func TestPipelineEscalatesWithReasonWhenGeneratorUnavailable(t *testing.T) {
	pipeline := NewPipeline(Config{
		Retrievers:          []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "restart printer", Score: 0.9}}}},
		Compressor:          Compressor{MaxRunes: 100},
		CRAG:                fakeCRAG{verdict: VerdictRelevant},
		Generator:           &stubGenerator{err: &llmerr.ProviderError{Status: 503, RetryAfter: 2 * time.Second}},
		ConfidenceThreshold: 0.85,
		Workers:             1,
	})

	result, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "printer offline"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Decision != DecisionEscalate {
		t.Fatalf("Decision = %q, want escalate", result.Decision)
	}
	if result.Reason != ReasonLLMUnavailable {
		t.Fatalf("Reason = %q, want llm_unavailable", result.Reason)
	}
	if result.RetryAfterMs != 2000 {
		t.Fatalf("RetryAfterMs = %d, want 2000", result.RetryAfterMs)
	}
}

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

type stubBudget struct {
	allow int
	calls int
}

func (b *stubBudget) Begin(ctx context.Context) context.Context { return ctx }

func (b *stubBudget) Acquire(_ context.Context, _ int64, _ string) (bool, error) {
	b.calls++
	if b.allow < 0 {
		return true, nil
	}
	return b.calls <= b.allow, nil
}

func TestPipelineBudgetExhaustionDegradesToEscalate(t *testing.T) {
	gen := &stubGenerator{draft: "draft"}
	pipeline := NewPipeline(Config{
		Retrievers:          []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "x"}}}},
		Reranker:            LexicalReranker{},
		Compressor:          Compressor{MaxRunes: 100},
		CRAG:                fakeCRAG{verdict: VerdictRelevant},
		Generator:           gen,
		Budget:              &stubBudget{allow: 0},
		ConfidenceThreshold: 0.85,
		Workers:             1,
	})
	res, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "help"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gen.calls != 0 {
		t.Fatalf("generator calls = %d, want 0 when budget exhausted", gen.calls)
	}
	if res.Decision != DecisionEscalate {
		t.Fatalf("Decision = %q, want escalate when no LLM budget (CRAG skipped, no draft)", res.Decision)
	}
}

func TestPipelineBudgetDeniedCriticDowngradesToDraft(t *testing.T) {
	critic := &stubCritic{res: CritiqueResult{Supported: true, Send: true}}
	pipeline := NewPipeline(Config{
		Retrievers:          []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "x"}}}},
		Reranker:            LexicalReranker{},
		Compressor:          Compressor{MaxRunes: 100},
		CRAG:                fakeCRAG{verdict: VerdictRelevant},
		Generator:           &stubGenerator{draft: "an answer"},
		Critic:              critic,
		Budget:              &stubBudget{allow: 3},
		ConfidenceThreshold: 0.85,
		Workers:             1,
	})
	res, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "help"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if critic.calls != 0 {
		t.Fatalf("critic calls = %d, want 0 (critic over budget)", critic.calls)
	}
	if res.Decision != DecisionDraft {
		t.Fatalf("Decision = %q, want draft (auto cannot stand without an in-budget critic)", res.Decision)
	}
}

func TestPipelineCriticErrorDowngradesToDraft(t *testing.T) {
	pipeline := NewPipeline(Config{
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
	if res.Decision != DecisionDraft {
		t.Fatalf("Decision = %q, want draft (critic error must fail closed)", res.Decision)
	}
	if res.Critique.Send {
		t.Fatal("Critique.Send = true, want false after critic error")
	}
}

func TestPipelineSkipsGeneratorOnEscalate(t *testing.T) {
	gen := &stubGenerator{draft: "should not appear"}
	pipeline := NewPipeline(Config{
		Retrievers:          []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "x"}}}},
		Reranker:            LexicalReranker{},
		Compressor:          Compressor{MaxRunes: 100},
		CRAG:                fakeCRAG{result: CRAGResult{Verdict: VerdictPartial, Confidence: 0.3}},
		Generator:           gen,
		ConfidenceThreshold: 0.85,
		Workers:             1,
	})
	res, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "help"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Decision != DecisionEscalate {
		t.Fatalf("Decision = %q, want escalate (confidence 0.3)", res.Decision)
	}
	if gen.calls != 0 {
		t.Fatalf("generator calls = %d, want 0 on escalate (no wasted LLM call)", gen.calls)
	}
}
