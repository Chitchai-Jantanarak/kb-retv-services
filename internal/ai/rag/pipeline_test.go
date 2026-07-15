package rag

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEvidenceContentJoinsGradedSet(t *testing.T) {
	cands := []Candidate{
		{ID: "1", Content: "alpha"},
		{ID: "2", Content: "bravo"},
		{ID: "3", Content: "charlie"},
	}
	out := evidenceContent(cands)
	for _, want := range []string{"alpha", "bravo", "charlie"} {
		if !strings.Contains(out, want) {
			t.Fatalf("evidence set missing %q: %q", want, out)
		}
	}
}

func TestEvidenceContentEmpty(t *testing.T) {
	if evidenceContent(nil) != "" {
		t.Fatal("no candidates -> empty evidence")
	}
}

func TestWillGenerateGuards(t *testing.T) {
	p := &Pipeline{}
	if willGenerate(p, DecisionAuto, VerdictRelevant, nil) {
		t.Fatal("no candidates -> no generation")
	}
	if willGenerate(p, DecisionEscalate, VerdictRelevant, []Candidate{{ID: "1"}}) {
		t.Fatal("escalate -> no generation")
	}
	if willGenerate(p, DecisionAuto, VerdictIrrelevant, []Candidate{{ID: "1"}}) {
		t.Fatal("irrelevant -> no generation")
	}
	if willGenerate(p, DecisionAuto, VerdictRelevant, []Candidate{{ID: "1"}}) {
		t.Fatal("nil generator -> no generation")
	}
}

func TestPipelineRunCachesRerankedCompressedResult(t *testing.T) {
	cache := newMemoryCache()
	retriever := &recordingRetriever{
		candidates: []Candidate{
			{ID: "2", Title: "General note", Content: "general support", Score: 0.1},
			{ID: "1", Title: "Printer offline checklist", Content: "printer offline network queue power", Score: 0.1},
		},
	}
	pipeline := NewPipeline(Config{
		Extractor:  DefaultExtractor{},
		Retrievers: []Retriever{retriever},
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 32},
		Cache:      cache,
		CacheTTL:   time.Minute,
		Workers:    2,
	})
	query := Query{
		CompanyID: 1,
		Text:      "printer offline",
		Limit:     2,
	}

	first, err := pipeline.Run(context.Background(), query)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	second, err := pipeline.Run(context.Background(), query)
	if err != nil {
		t.Fatalf("cached Run() error = %v", err)
	}

	if retriever.calls != 1 {
		t.Fatalf("retriever calls = %d, want 1", retriever.calls)
	}
	if first.Candidates[0].ID != "1" {
		t.Fatalf("top candidate = %q, want 1", first.Candidates[0].ID)
	}
	if second.Candidates[0].ID != "1" {
		t.Fatalf("cached top candidate = %q, want 1", second.Candidates[0].ID)
	}
	if len([]rune(first.Context)) > 32 {
		t.Fatalf("context runes = %d, want <= 32", len([]rune(first.Context)))
	}
	if retriever.last.CompanyID != 1 {
		t.Fatalf("company_id = %d, want 1", retriever.last.CompanyID)
	}
	if first.Meta.Intent != "troubleshooting" {
		t.Fatalf("intent = %q, want troubleshooting", first.Meta.Intent)
	}
}

func TestPipelineUsesPlanForCacheAndRetrieverQuery(t *testing.T) {
	cache := newMemoryCache()
	retriever := &recordingRetriever{
		candidates: []Candidate{{ID: "1", Title: "Printer", Content: "printer offline fix", Score: 0.9}},
	}
	pipeline := NewPipeline(Config{
		Extractor:  DefaultExtractor{},
		Retrievers: []Retriever{retriever},
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 100},
		CRAG:       fakeCRAG{verdict: VerdictRelevant},
		Cache:      cache,
		CacheTTL:   time.Minute,
		Workers:    1,
	})

	first, err := pipeline.Run(context.Background(), Query{
		CompanyID: 1,
		Text:      "  Printer    OFFLINE  ",
	})
	if err != nil {
		t.Fatalf("Run first: %v", err)
	}
	second, err := pipeline.Run(context.Background(), Query{
		CompanyID: 1,
		Text:      "printer offline",
	})
	if err != nil {
		t.Fatalf("Run second: %v", err)
	}

	if retriever.calls != 1 {
		t.Fatalf("retriever calls = %d, want 1 because normalized queries share cache", retriever.calls)
	}
	if retriever.last.Text != "Printer OFFLINE" {
		t.Fatalf("retriever query text = %q, want planned retrieval text", retriever.last.Text)
	}
	if !second.CacheHit {
		t.Fatal("second result CacheHit = false, want true")
	}
	if first.Candidates[0].ID != second.Candidates[0].ID {
		t.Fatalf("cached candidate mismatch: first=%+v second=%+v", first.Candidates, second.Candidates)
	}
}

func TestPipelineDoesNotCallRetrieversWhenPlannerRejectsQuery(t *testing.T) {
	retriever := &recordingRetriever{
		candidates: []Candidate{{ID: "1", Content: "should not be used"}},
	}
	pipeline := NewPipeline(Config{
		Planner:    rejectingPlanner{},
		Retrievers: []Retriever{retriever},
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 100},
		Workers:    1,
	})

	_, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "valid before planner"})
	if err == nil {
		t.Fatal("Run err = nil, want planner error")
	}
	if retriever.calls != 0 {
		t.Fatalf("retriever calls = %d, want 0 after planner rejection", retriever.calls)
	}
}

func TestPipelinePlannerPreservesCompanyContextForRetrievers(t *testing.T) {
	retriever := &recordingRetriever{
		candidates: []Candidate{{ID: "1", Content: "x"}},
	}
	pipeline := NewPipeline(Config{
		Extractor:  DefaultExtractor{},
		Retrievers: []Retriever{retriever},
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 100},
		Workers:    1,
	})

	_, err := pipeline.Run(context.Background(), Query{
		CompanyID: 42,
		Text:      "  printer   offline  ",
		Limit:     20,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if retriever.last.CompanyID != 42 {
		t.Fatalf("retriever company_id = %d, want 42", retriever.last.CompanyID)
	}
	if retriever.last.Limit != maxQueryPlanLimit {
		t.Fatalf("retriever limit = %d, want planned cap %d", retriever.last.Limit, maxQueryPlanLimit)
	}
}

func TestPipelineRetrievesEveryPlannedQuery(t *testing.T) {
	retriever := &recordingRetriever{
		candidates: []Candidate{{ID: "1", Content: "x"}},
	}
	pipeline := NewPipeline(Config{
		Planner: plannedQueriesPlanner{
			queries: []PlannedQuery{
				{Kind: PlannedQueryOriginal, Text: "printer offline"},
				{Kind: "hyde", Text: "printer cannot connect to network"},
				{Kind: "fusion", Text: "queue stuck power cycle"},
			},
		},
		Retrievers: []Retriever{retriever},
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 100},
		Workers:    1,
	})

	_, err := pipeline.Run(context.Background(), Query{
		CompanyID: 9,
		Text:      "printer offline",
		Limit:     3,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := retriever.texts()
	want := []string{"printer offline", "printer cannot connect to network", "queue stuck power cycle"}
	if len(got) != len(want) {
		t.Fatalf("retrieved texts = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("retrieved texts = %#v, want %#v", got, want)
		}
	}
}

func TestPipelineRetrievesGraphAnchorsFromPlan(t *testing.T) {
	retriever := &recordingRetriever{
		candidates: []Candidate{{ID: "text", Content: "text candidate"}},
	}
	graph := &recordingGraphRetriever{
		candidates: []Candidate{{ID: "graph", Content: "graph candidate", Source: "graph"}},
	}
	pipeline := NewPipeline(Config{
		Planner: graphAnchorPlanner{
			anchors: []GraphAnchor{{Kind: GraphAnchorSymptom, ID: 55}},
		},
		Retrievers:     []Retriever{retriever},
		GraphRetriever: graph,
		Reranker:       LexicalReranker{},
		Compressor:     Compressor{MaxRunes: 100},
		Workers:        1,
	})

	result, err := pipeline.Run(context.Background(), Query{
		CompanyID: 9,
		Text:      "printer jams",
		Limit:     3,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if graph.calls != 1 {
		t.Fatalf("graph calls = %d, want 1", graph.calls)
	}
	if graph.lastPlan.GraphAnchors[0].ID != 55 {
		t.Fatalf("graph plan = %+v", graph.lastPlan)
	}
	if len(result.Candidates) == 0 || result.Candidates[0].ID != "graph" {
		t.Fatalf("candidates = %+v, want graph candidate boosted first", result.Candidates)
	}
}

func TestPipelineSkipsFailedGraphRetriever(t *testing.T) {
	retriever := &recordingRetriever{
		candidates: []Candidate{{ID: "text", Content: "text candidate", Source: "kb"}},
	}
	graph := &recordingGraphRetriever{err: errors.New("memgraph down")}
	pipeline := NewPipeline(Config{
		Planner: graphAnchorPlanner{
			anchors: []GraphAnchor{{Kind: GraphAnchorSymptom, ID: 55}},
		},
		Retrievers:     []Retriever{retriever},
		GraphRetriever: graph,
		Reranker:       LexicalReranker{},
		Compressor:     Compressor{MaxRunes: 100},
		Workers:        1,
	})

	result, err := pipeline.Run(context.Background(), Query{
		CompanyID: 9,
		Text:      "printer jams",
		Limit:     3,
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want graph failure swallowed", err)
	}
	if graph.calls != 1 {
		t.Fatalf("graph calls = %d, want 1", graph.calls)
	}
	if len(result.Candidates) == 0 || result.Candidates[0].ID != "text" {
		t.Fatalf("candidates = %+v, want fallback text candidate", result.Candidates)
	}
}

func TestPipelineRunSkipsFailedRetriever(t *testing.T) {
	pipeline := NewPipeline(Config{
		Extractor: DefaultExtractor{},
		Retrievers: []Retriever{
			&recordingRetriever{err: errors.New("qdrant down")},
			&recordingRetriever{candidates: []Candidate{{ID: "kb-1", Title: "Printer", Content: "printer offline fix", Score: 0.8}}},
		},
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 100},
		Workers:    2,
	})

	res, err := pipeline.Run(context.Background(), Query{
		CompanyID: 1,
		Text:      "printer offline",
	})
	if err != nil {
		t.Fatalf("Run errored despite a healthy retriever: %v", err)
	}
	if len(res.Candidates) == 0 {
		t.Fatal("expected candidates from the healthy retriever")
	}
}

func TestPipelineRunDeduplicatesCandidatesByHighestScore(t *testing.T) {
	pipeline := NewPipeline(Config{
		Extractor: DefaultExtractor{},
		Retrievers: []Retriever{
			&recordingRetriever{candidates: []Candidate{{ID: "same", Title: "Old", Content: "old", Score: 0.2}}},
			&recordingRetriever{candidates: []Candidate{{ID: "same", Title: "New", Content: "new printer offline", Score: 0.9}}},
		},
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 100},
		Workers:    2,
	})

	result, err := pipeline.Run(context.Background(), Query{
		CompanyID: 1,
		Text:      "printer offline",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(result.Candidates) != 1 {
		t.Fatalf("candidates = %d, want 1", len(result.Candidates))
	}
	if result.Candidates[0].Title != "New" {
		t.Fatalf("candidate title = %q, want New", result.Candidates[0].Title)
	}
}

func TestPipelineRunEscalatesWhenNoCandidates(t *testing.T) {
	pipeline := NewPipeline(Config{
		Extractor:  DefaultExtractor{},
		Retrievers: []Retriever{&recordingRetriever{candidates: nil}},
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 100},
		Workers:    1,
	})

	result, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "unknown topic"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Decision != DecisionEscalate {
		t.Fatalf("Decision = %q, want escalate", result.Decision)
	}
	if result.Confidence != 0 {
		t.Fatalf("Confidence = %v, want 0", result.Confidence)
	}
}

func TestPipelineRunAutoSendsAboveThreshold(t *testing.T) {
	pipeline := NewPipeline(Config{
		Extractor:           DefaultExtractor{},
		Retrievers:          []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Title: "Fix", Content: "solution", Score: 0.9}}}},
		Reranker:            LexicalReranker{},
		Compressor:          Compressor{MaxRunes: 100},
		CRAG:                fakeCRAG{verdict: VerdictRelevant},
		Generator:           &stubGenerator{draft: "answer"},
		Workers:             1,
		ConfidenceThreshold: 0.85,
	})

	result, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "help"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Decision != DecisionAuto {
		t.Fatalf("Decision = %q, want auto", result.Decision)
	}
	if result.Confidence < 0.85 {
		t.Fatalf("Confidence = %v, want >= 0.85", result.Confidence)
	}
}

func TestPipelineUngradedCRAGNeverAutoSends(t *testing.T) {
	pipeline := NewPipeline(Config{
		Extractor:           DefaultExtractor{},
		Retrievers:          []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Title: "Fix", Content: "solution", Score: 0.95}}}},
		Reranker:            LexicalReranker{},
		Compressor:          Compressor{MaxRunes: 100},
		CRAG:                PassthroughCRAG{},
		Generator:           &stubGenerator{draft: "draft"},
		Workers:             1,
		ConfidenceThreshold: 0.85,
	})

	result, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "help"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Decision == DecisionAuto {
		t.Fatalf("Decision = auto, want draft/escalate: ungraded CRAG must never auto-send")
	}
	if result.CRAG.Graded {
		t.Fatal("CRAG.Graded = true, want false for passthrough")
	}
}

func TestPipelineUsesCRAGResultConfidence(t *testing.T) {
	pipeline := NewPipeline(Config{
		Extractor: DefaultExtractor{},
		Retrievers: []Retriever{
			&recordingRetriever{candidates: []Candidate{{ID: "1", Title: "Fix", Content: "solution", Score: 0.9}}},
		},
		Reranker:   LexicalReranker{},
		Compressor: Compressor{MaxRunes: 100},
		CRAG: fakeCRAG{result: CRAGResult{
			Verdict:    VerdictRelevant,
			Confidence: 0.62,
			Missing:    "serial number",
		}},
		Workers: 1,
	})

	result, err := pipeline.Run(context.Background(), Query{CompanyID: 1, Text: "help"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Confidence != 0.62 {
		t.Fatalf("Confidence = %v, want CRAG confidence 0.62", result.Confidence)
	}
	if result.CRAG.Missing != "serial number" {
		t.Fatalf("CRAG missing = %q, want structured missing reason", result.CRAG.Missing)
	}
}

func TestPipelineKnowledgeGapRecording(t *testing.T) {
	cases := []struct {
		name      string
		verdict   CRAGVerdict
		wantCalls int
	}{
		{"records_on_irrelevant", VerdictIrrelevant, 1},
		{"no_record_on_relevant", VerdictRelevant, 0},
		{"no_record_on_partial", VerdictPartial, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := &recordingGap{}
			pipeline := NewPipeline(Config{
				Extractor:    DefaultExtractor{},
				Retrievers:   []Retriever{&recordingRetriever{candidates: []Candidate{{ID: "1", Content: "x"}}}},
				Reranker:     LexicalReranker{},
				CRAG:         fakeCRAG{verdict: tc.verdict},
				Compressor:   Compressor{MaxRunes: 100},
				Workers:      1,
				KnowledgeGap: recorder,
			})

			_, err := pipeline.Run(context.Background(), Query{CompanyID: 7, Text: "broken thing"})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if recorder.calls != tc.wantCalls {
				t.Fatalf("recorder calls = %d, want %d", recorder.calls, tc.wantCalls)
			}
			if tc.wantCalls > 0 {
				if recorder.lastCompanyID != 7 {
					t.Fatalf("companyID = %d, want 7", recorder.lastCompanyID)
				}
				if recorder.lastText != "broken thing" {
					t.Fatalf("text = %q, want %q", recorder.lastText, "broken thing")
				}
			}
		})
	}
}

func TestPipelineKnowledgeGapRecordsWhenNoCandidates(t *testing.T) {
	recorder := &recordingGap{}
	pipeline := NewPipeline(Config{
		Extractor:    DefaultExtractor{},
		Retrievers:   []Retriever{&recordingRetriever{candidates: nil}},
		Reranker:     LexicalReranker{},
		CRAG:         fakeCRAG{verdict: VerdictIrrelevant},
		Compressor:   Compressor{MaxRunes: 100},
		Workers:      1,
		KnowledgeGap: recorder,
	})

	_, err := pipeline.Run(context.Background(), Query{CompanyID: 7, Text: "no hits"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if recorder.calls != 1 {
		t.Fatalf("recorder calls = %d, want 1 (gap must be logged even with empty candidates)", recorder.calls)
	}
}

func TestPipelineKnowledgeGapFailureCases(t *testing.T) {
	cases := []struct {
		name     string
		recorder *recordingGap
	}{
		{"recorder_returns_error", &recordingGap{err: errors.New("db down")}},
		{"nil_recorder_is_safe", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{
				Extractor:  DefaultExtractor{},
				Retrievers: []Retriever{&recordingRetriever{candidates: nil}},
				Reranker:   LexicalReranker{},
				CRAG:       fakeCRAG{verdict: VerdictIrrelevant},
				Compressor: Compressor{MaxRunes: 100},
				Workers:    1,
			}
			if tc.recorder != nil {
				cfg.KnowledgeGap = tc.recorder
			}
			pipeline := NewPipeline(cfg)

			result, err := pipeline.Run(context.Background(), Query{CompanyID: 7, Text: "fail"})
			if err != nil {
				t.Fatalf("Run() error = %v (recorder failures must be swallowed, nil must be safe)", err)
			}
			if result.Decision != DecisionEscalate {
				t.Fatalf("Decision = %q, want escalate", result.Decision)
			}
		})
	}
}

type fakeCRAG struct {
	verdict CRAGVerdict
	result  CRAGResult
	err     error
}

func (f fakeCRAG) Grade(_ context.Context, _, _ string) (CRAGResult, error) {
	res := f.result
	if res.Verdict == 0 && res.Confidence == 0 && res.Missing == "" {
		res = CRAGResult{Verdict: f.verdict}
	}
	res.Graded = true
	return res, f.err
}

type recordingGap struct {
	mu            sync.Mutex
	calls         int
	lastCompanyID int64
	lastText      string
	err           error
}

func (r *recordingGap) Record(_ context.Context, companyID int64, queryText string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.lastCompanyID = companyID
	r.lastText = queryText
	return r.err
}

type recordingRetriever struct {
	mu         sync.Mutex
	calls      int
	last       Query
	queries    []Query
	candidates []Candidate
	err        error
}

func (r *recordingRetriever) Retrieve(ctx context.Context, query Query, meta Meta) ([]Candidate, error) {
	r.mu.Lock()
	r.calls++
	r.last = query
	r.queries = append(r.queries, query)
	r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.err != nil {
		return nil, r.err
	}
	return r.candidates, nil
}

func (r *recordingRetriever) texts() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.queries))
	for _, q := range r.queries {
		out = append(out, q.Text)
	}
	return out
}

type rejectingPlanner struct{}

func (rejectingPlanner) Plan(context.Context, Query) (QueryPlan, error) {
	return QueryPlan{}, errors.New("planner rejected query")
}

type plannedQueriesPlanner struct {
	queries []PlannedQuery
}

func (p plannedQueriesPlanner) Plan(_ context.Context, query Query) (QueryPlan, error) {
	return QueryPlan{
		OriginalText:     query.Text,
		RetrievalText:    query.Text,
		CanonicalText:    query.Text,
		EffectiveLimit:   query.Limit,
		RetrievalQueries: p.queries,
		SkipReasons:      map[string]string{},
	}, nil
}

type graphAnchorPlanner struct {
	anchors []GraphAnchor
}

func (p graphAnchorPlanner) Plan(_ context.Context, query Query) (QueryPlan, error) {
	return QueryPlan{
		OriginalText:   query.Text,
		RetrievalText:  query.Text,
		CanonicalText:  query.Text,
		EffectiveLimit: query.Limit,
		RetrievalQueries: []PlannedQuery{
			{Kind: PlannedQueryOriginal, Text: query.Text},
		},
		GraphAnchors: p.anchors,
		SkipReasons:  map[string]string{},
	}, nil
}

type recordingGraphRetriever struct {
	calls      int
	lastPlan   QueryPlan
	candidates []Candidate
	err        error
}

func (r *recordingGraphRetriever) RetrieveGraph(_ context.Context, _ Query, plan QueryPlan) ([]Candidate, error) {
	r.calls++
	r.lastPlan = plan
	if r.err != nil {
		return nil, r.err
	}
	return r.candidates, nil
}
