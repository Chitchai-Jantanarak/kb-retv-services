package rag

import (
	"context"
	"testing"
)

func TestPipelineDebugStageTimings(t *testing.T) {
	reranker := &recordingReranker{}
	critic := &stubCritic{res: CritiqueResult{Supported: true, Send: true}}
	pipeline := NewPipeline(Config{
		Retrievers:          []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "restart printer", Score: 0.8}}}},
		Reranker:            reranker,
		Compressor:          Compressor{MaxRunes: 100},
		CRAG:                fakeCRAG{verdict: VerdictRelevant},
		Generator:           &stubGenerator{draft: "draft"},
		Critic:              critic,
		ConfidenceThreshold: 0.85,
		Workers:             1,
	})

	result, err := pipeline.Run(context.Background(), Query{
		CompanyID: 1,
		Text:      "printer offline",
		Debug:     true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, stage := range []string{"extract", "retrieve", "rerank", "crag", "generate", "critique"} {
		if _, ok := result.StageTimingsMS[stage]; !ok {
			t.Fatalf("StageTimingsMS missing %q: %+v", stage, result.StageTimingsMS)
		}
	}
	if reranker.calls != 1 {
		t.Fatalf("reranker calls = %d, want 1", reranker.calls)
	}
	if critic.calls != 1 {
		t.Fatalf("critic calls = %d, want 1", critic.calls)
	}
}

func TestPipelineDebugTraceSummarizesPlanRetrievalAndQuality(t *testing.T) {
	pipeline := NewPipeline(Config{
		Retrievers: []Retriever{&recordingRetriever{candidates: []Candidate{
			{ID: "1", Content: "restart printer", Source: "knowledge", Score: 0.8},
		}}},
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 100},
		CRAG: fakeCRAG{result: CRAGResult{
			Verdict:    VerdictPartial,
			Confidence: 0.7,
			Missing:    "serial number",
		}},
		Generator:           &stubGenerator{draft: "draft"},
		Critic:              &stubCritic{res: CritiqueResult{Supported: false, Send: false}},
		ConfidenceThreshold: 0.85,
		Workers:             1,
	})

	result, err := pipeline.Run(context.Background(), Query{
		CompanyID: 1,
		Text:      "printer offline",
		Mode:      ModeDebug,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	trace := result.DebugTrace
	if trace == nil {
		t.Fatal("DebugTrace = nil, want trace in debug mode")
	}
	if len(trace.Plan.Terms) == 0 {
		t.Fatalf("trace plan terms = %+v, want non-empty", trace.Plan.Terms)
	}
	if trace.Retrieval.Candidates != 1 || trace.Retrieval.TopSource != "knowledge" {
		t.Fatalf("trace retrieval = %+v, want one knowledge candidate", trace.Retrieval)
	}
	if trace.Quality.CRAGVerdict != "partial" || trace.Quality.CRAGMissing != "serial number" {
		t.Fatalf("trace quality CRAG = %+v, want partial with missing reason", trace.Quality)
	}
	if trace.Quality.Decision != string(DecisionDraft) || trace.Quality.CriticRan {
		t.Fatalf("trace quality decision = %+v, want draft without critic", trace.Quality)
	}
}

func TestPipelineOmitsStageTimingsByDefault(t *testing.T) {
	pipeline := NewPipeline(Config{
		Retrievers: []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "restart printer", Score: 0.8}}}},
		Reranker:   &recordingReranker{},
		Compressor: Compressor{MaxRunes: 100},
		CRAG:       fakeCRAG{verdict: VerdictRelevant},
		Generator:  &stubGenerator{draft: "draft"},
		Critic:     &stubCritic{res: CritiqueResult{Supported: true, Send: true}},
		Workers:    1,
	})

	result, err := pipeline.Run(context.Background(), Query{
		CompanyID: 1,
		Text:      "printer offline",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.StageTimingsMS != nil {
		t.Fatalf("StageTimingsMS = %+v, want nil without debug", result.StageTimingsMS)
	}
	if result.DebugTrace != nil {
		t.Fatalf("DebugTrace = %+v, want nil without debug", result.DebugTrace)
	}
}

func TestPipelineFastDraftSkipsCriticAndSingleCandidateRerank(t *testing.T) {
	reranker := &recordingReranker{}
	generator := &stubGenerator{draft: "draft"}
	critic := &stubCritic{res: CritiqueResult{Supported: false, Send: false}}
	pipeline := NewPipeline(Config{
		Retrievers:          []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "restart printer", Score: 0.8}}}},
		Reranker:            reranker,
		Compressor:          Compressor{MaxRunes: 100},
		CRAG:                fakeCRAG{verdict: VerdictRelevant},
		Generator:           generator,
		Critic:              critic,
		ConfidenceThreshold: 0.85,
		Workers:             1,
	})

	result, err := pipeline.Run(context.Background(), Query{
		CompanyID: 1,
		Text:      "printer offline",
		Mode:      ModeFastDraft,
		Debug:     true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if generator.calls != 1 {
		t.Fatalf("generator calls = %d, want 1", generator.calls)
	}
	if critic.calls != 0 {
		t.Fatalf("critic calls = %d, want 0 in fast_draft", critic.calls)
	}
	if reranker.calls != 0 {
		t.Fatalf("reranker calls = %d, want 0 for one candidate in fast_draft", reranker.calls)
	}
	if _, ok := result.StageTimingsMS["critique"]; ok {
		t.Fatalf("critique timing recorded in fast_draft: %+v", result.StageTimingsMS)
	}
	if _, ok := result.StageTimingsMS["rerank"]; ok {
		t.Fatalf("rerank timing recorded for one candidate in fast_draft: %+v", result.StageTimingsMS)
	}
	if result.Decision != DecisionAuto {
		t.Fatalf("Decision = %q, want auto", result.Decision)
	}
}

func TestPipelineFastDraftUsesCheapRetrievalBeforeExpensive(t *testing.T) {
	cheap := &costRecordingRetriever{
		cost:       RetrievalCostCheap,
		candidates: []Candidate{{ID: "cheap", Source: "cheap", Content: "cheap answer", Score: 0.7}},
	}
	expensive := &costRecordingRetriever{
		cost:       RetrievalCostExpensive,
		candidates: []Candidate{{ID: "expensive", Source: "expensive", Content: "expensive answer", Score: 0.9}},
	}
	pipeline := NewPipeline(Config{
		Retrievers: []Retriever{cheap, expensive},
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 100},
		CRAG:       fakeCRAG{verdict: VerdictRelevant},
		Workers:    2,
	})

	result, err := pipeline.Run(context.Background(), Query{
		CompanyID: 1,
		Text:      "printer offline",
		Mode:      ModeFastDraft,
		Debug:     true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if cheap.calls != 1 {
		t.Fatalf("cheap calls = %d, want 1", cheap.calls)
	}
	if expensive.calls != 0 {
		t.Fatalf("expensive calls = %d, want 0 when cheap retrieval has hits", expensive.calls)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].ID != "cheap" {
		t.Fatalf("candidates = %+v, want only cheap candidate", result.Candidates)
	}
	if _, ok := result.StageTimingsMS["retrieve_cheap"]; !ok {
		t.Fatalf("retrieve_cheap timing missing: %+v", result.StageTimingsMS)
	}
	if _, ok := result.StageTimingsMS["retrieve_expensive"]; ok {
		t.Fatalf("retrieve_expensive timing recorded despite cheap hit: %+v", result.StageTimingsMS)
	}
}

func TestPipelineFastDraftFallsBackToExpensiveRetrievalWhenCheapMisses(t *testing.T) {
	cheap := &costRecordingRetriever{cost: RetrievalCostCheap}
	expensive := &costRecordingRetriever{
		cost:       RetrievalCostExpensive,
		candidates: []Candidate{{ID: "expensive", Source: "expensive", Content: "expensive answer", Score: 0.9}},
	}
	pipeline := NewPipeline(Config{
		Retrievers: []Retriever{cheap, expensive},
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 100},
		CRAG:       fakeCRAG{verdict: VerdictRelevant},
		Workers:    2,
	})

	result, err := pipeline.Run(context.Background(), Query{
		CompanyID: 1,
		Text:      "printer offline",
		Mode:      ModeFastDraft,
		Debug:     true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if cheap.calls != 1 {
		t.Fatalf("cheap calls = %d, want 1", cheap.calls)
	}
	if expensive.calls != 1 {
		t.Fatalf("expensive calls = %d, want 1 after cheap retrieval misses", expensive.calls)
	}
	if len(result.Candidates) != 1 || result.Candidates[0].ID != "expensive" {
		t.Fatalf("candidates = %+v, want expensive fallback candidate", result.Candidates)
	}
	if _, ok := result.StageTimingsMS["retrieve_expensive"]; !ok {
		t.Fatalf("retrieve_expensive timing missing after fallback: %+v", result.StageTimingsMS)
	}
}

func TestFastDraftSkipsLLMRerank(t *testing.T) {
	q := Query{Mode: ModeFastDraft}
	many := []Candidate{{ID: "1"}, {ID: "2"}, {ID: "3"}}
	if shouldRerank(q, many) {
		t.Fatal("fast_draft must skip LLM rerank even with many candidates")
	}
}

func TestFastDraftKeepsCRAGForDecision(t *testing.T) {
	q := Query{Mode: ModeFastDraft}
	many := []Candidate{{ID: "1"}, {ID: "2"}}
	if !shouldRunCRAG(q, many) {
		t.Fatal("fast_draft keeps CRAG; skipping it forces Graded=false -> escalate -> no draft")
	}
}

func TestFullReviewStillReranks(t *testing.T) {
	q := Query{Mode: ModeFullReview}
	if !shouldRerank(q, []Candidate{{ID: "1"}}) {
		t.Fatal("full_review must keep rerank")
	}
}

type recordingReranker struct {
	calls int
}

func (r *recordingReranker) Rerank(_ context.Context, _ Query, _ Meta, candidates []Candidate) ([]Candidate, error) {
	r.calls++
	return candidates, nil
}

type costRecordingRetriever struct {
	cost       RetrievalCost
	calls      int
	candidates []Candidate
}

func (r *costRecordingRetriever) RetrievalCost() RetrievalCost {
	return r.cost
}

func (r *costRecordingRetriever) Retrieve(_ context.Context, _ Query, _ Meta) ([]Candidate, error) {
	r.calls++
	return r.candidates, nil
}
