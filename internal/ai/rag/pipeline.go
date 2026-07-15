package rag

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
	"github.com/my/app/internal/shared/logger"
	"github.com/my/app/pkg/concurrency"
)

const defaultConfidenceThreshold = 0.85

type Pipeline struct {
	planner             Planner
	extractor           Extractor
	retrievers          []Retriever
	graphRetriever      GraphRetriever
	contextAssembler    ContextAssembler
	reranker            Reranker
	compressor          Compressor
	crag                CRAG
	generator           Generator
	critic              Critic
	confidenceThreshold float64
	thresholdFor        ThresholdForCompany
	cache               cacheConfig
	knowledgeGap        ports.KnowledgeGapRecorder
	budget              Budget
	workers             int
}

type cacheConfig struct {
	store ports.Cache
	ttl   time.Duration
}

func NewPipeline(cfg Config) *Pipeline {
	extractor := cfg.Extractor
	if extractor == nil {
		extractor = DefaultExtractor{}
	}
	planner := cfg.Planner
	if planner == nil {
		planner = DefaultQueryPlanner{Extractor: extractor}
	}
	reranker := cfg.Reranker
	if reranker == nil {
		reranker = LexicalReranker{}
	}
	crag := cfg.CRAG
	if crag == nil {
		crag = PassthroughCRAG{}
	}
	threshold := cfg.ConfidenceThreshold
	if threshold <= 0 {
		threshold = defaultConfidenceThreshold
	}
	workers := cfg.Workers
	if workers <= 0 {
		workers = 1
	}
	return &Pipeline{
		planner:             planner,
		extractor:           extractor,
		retrievers:          cfg.Retrievers,
		graphRetriever:      cfg.GraphRetriever,
		contextAssembler:    cfg.ContextAssembler,
		reranker:            reranker,
		compressor:          cfg.Compressor,
		crag:                crag,
		generator:           cfg.Generator,
		critic:              cfg.Critic,
		confidenceThreshold: threshold,
		thresholdFor:        cfg.ConfidenceThresholdFor,
		cache: cacheConfig{
			store: cfg.Cache,
			ttl:   cfg.CacheTTL,
		},
		knowledgeGap: cfg.KnowledgeGap,
		budget:       cfg.Budget,
		workers:      workers,
	}
}

func (p *Pipeline) Run(ctx context.Context, query Query) (Result, error) {
	if err := validateQuery(query); err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	ctx = ctxkey.WithCompanyID(ctx, query.CompanyID)
	if p.budget != nil {
		ctx = p.budget.Begin(ctx)
	}

	timings := newStageTimings(query)

	var plan QueryPlan
	if err := timed(timings, "extract", func() error {
		var err error
		plan, err = p.planner.Plan(ctx, query)
		return err
	}); err != nil {
		return Result{}, err
	}
	plannedQuery := applyQueryPlan(query, plan)

	key := cacheKeyForPlan(query, plan)
	if !wantsTimings(query) {
		cached, ok, err := getCached(ctx, p.cache.store, key)
		if err != nil {
			return Result{}, err
		}
		if ok {
			cached.Decision = decisionFor(cached.Confidence, p.threshold(ctx, plannedQuery), cached.CRAG.Graded)
			if cached.Decision == DecisionAuto && strings.TrimSpace(cached.Draft) == "" {
				cached.Decision = DecisionEscalate
			}
			return cached, nil
		}
	}

	var candidates []Candidate
	if err := timed(timings, "retrieve", func() error {
		var err error
		candidates, err = p.retrieveCandidates(ctx, plannedQuery, plan, timings)
		return err
	}); err != nil {
		return Result{}, err
	}
	if p.graphRetriever != nil && len(plan.GraphAnchors) > 0 {
		if err := timed(timings, "retrieve_graph", func() error {
			graphCandidates, err := p.graphRetriever.RetrieveGraph(ctx, plannedQuery, plan)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return err
				}
				logger.FromContext(ctx).Warn("graph retriever failed; skipping", zap.Error(err))
				return nil
			}
			candidates = rrfMerge([][]Candidate{graphCandidates, candidates})
			return nil
		}); err != nil {
			return Result{}, err
		}
	}
	contextAssembled := false
	if p.contextAssembler != nil && len(candidates) > 0 {
		if err := timed(timings, "assemble_context", func() error {
			var err error
			candidates, err = p.contextAssembler.Assemble(ctx, plannedQuery, candidates)
			contextAssembled = true
			return err
		}); err != nil {
			return Result{}, err
		}
	}

	if shouldRerank(plannedQuery, candidates) && p.llmAllowed(ctx, query.CompanyID, "rerank") {
		if err := timed(timings, "rerank", func() error {
			var err error
			candidates, err = p.reranker.Rerank(ctx, plannedQuery, plan.Meta, candidates)
			return err
		}); err != nil {
			return Result{}, err
		}
	}
	if plannedQuery.Limit > 0 && len(candidates) > plannedQuery.Limit {
		candidates = candidates[:plannedQuery.Limit]
	}

	cragResult := CRAGResult{Verdict: VerdictIrrelevant}
	if shouldRunCRAG(plannedQuery, candidates) && p.llmAllowed(ctx, query.CompanyID, "crag") {
		if err := timed(timings, "crag", func() error {
			var err error
			cragResult, err = p.crag.Grade(ctx, plannedQuery.Text, topContent(candidates))
			return err
		}); err != nil {
			return Result{}, err
		}
	}
	verdict := cragResult.Verdict

	confidence := computeConfidence(cragResult, candidates)
	threshold := p.threshold(ctx, plannedQuery)
	decision := decisionFor(confidence, threshold, cragResult.Graded)

	p.recordKnowledgeGap(ctx, query, verdict)

	var draft string
	var genErr error
	if willGenerate(p, decision, verdict, candidates) && p.llmAllowed(ctx, query.CompanyID, "generate") {
		if err := timed(timings, "generate", func() error {
			draft, genErr = p.generateDraft(ctx, plannedQuery, verdict, decision, candidates)
			return nil
		}); err != nil {
			return Result{}, err
		}
	}
	critique := CritiqueResult{Supported: true, Send: true}
	criticRan := false
	if shouldRunCritic(plannedQuery, decision, draft) {
		if p.llmAllowed(ctx, query.CompanyID, "critic") {
			if err := timed(timings, "critique", func() error {
				criticRan = true
				critique = p.critiqueDraft(ctx, plannedQuery, draft, candidates)
				return nil
			}); err != nil {
				return Result{}, err
			}
			decision = applyCritiqueDecision(decision, critique)
		} else {
			decision = DecisionDraft
		}
	}
	if decision == DecisionAuto && strings.TrimSpace(draft) == "" {
		decision = DecisionEscalate
	}

	reason := ReasonNone
	var retryAfter time.Duration
	if decision == DecisionEscalate {
		reason, retryAfter = classifyEscalation(genErr)
	}

	result := Result{
		Meta:           plan.Meta,
		Candidates:     candidates,
		Context:        p.compressor.Compress(candidates),
		Confidence:     confidence,
		Decision:       decision,
		Reason:         reason,
		RetryAfterMs:   retryAfter.Milliseconds(),
		Draft:          draft,
		CRAG:           cragResult,
		Critique:       critique,
		StageTimingsMS: timings,
		DebugTrace:     debugTrace(plannedQuery, plan, candidates, contextAssembled, cragResult, confidence, threshold, decision, draft, criticRan),
	}
	cachedResult := result
	cachedResult.StageTimingsMS = nil
	cachedResult.DebugTrace = nil
	if err := setCached(ctx, p.cache.store, key, cachedResult, p.cache.ttl); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (p *Pipeline) retrieveCandidates(ctx context.Context, query Query, plan QueryPlan, timings map[string]int64) ([]Candidate, error) {
	meta := plan.Meta
	plannedQueries := retrievalQueriesForPlan(query, plan)
	if len(plannedQueries) > 1 {
		sets := make([][]Candidate, 0, len(plannedQueries))
		for _, planned := range plannedQueries {
			if strings.TrimSpace(planned.Text) == "" {
				continue
			}
			next := query
			next.Text = planned.Text
			got, err := p.retrieveCandidates(ctx, next, QueryPlan{Meta: meta}, timings)
			if err != nil {
				return nil, err
			}
			sets = append(sets, got)
		}
		return rrfMerge(sets), nil
	}

	if fastDraft(query) {
		cheap, expensive := splitRetrieversByCost(p.retrievers)
		if len(cheap) > 0 && len(expensive) > 0 {
			var candidates []Candidate
			if err := timed(timings, "retrieve_cheap", func() error {
				var err error
				candidates, err = p.retrieveFrom(ctx, cheap, query, meta)
				return err
			}); err != nil {
				return nil, err
			}
			if len(candidates) > 0 {
				return candidates, nil
			}
			if err := timed(timings, "retrieve_expensive", func() error {
				var err error
				candidates, err = p.retrieveFrom(ctx, expensive, query, meta)
				return err
			}); err != nil {
				return nil, err
			}
			return candidates, nil
		}
	}
	return p.retrieveFrom(ctx, p.retrievers, query, meta)
}

func retrievalQueriesForPlan(query Query, plan QueryPlan) []PlannedQuery {
	if len(plan.RetrievalQueries) == 0 {
		return []PlannedQuery{{Kind: PlannedQueryOriginal, Text: query.Text}}
	}
	seen := make(map[string]struct{}, len(plan.RetrievalQueries))
	out := make([]PlannedQuery, 0, len(plan.RetrievalQueries))
	for _, planned := range plan.RetrievalQueries {
		text := strings.TrimSpace(planned.Text)
		if text == "" {
			continue
		}
		key := canonicalQueryText(text)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		planned.Text = text
		out = append(out, planned)
	}
	if len(out) == 0 {
		return []PlannedQuery{{Kind: PlannedQueryOriginal, Text: query.Text}}
	}
	return out
}

func (p *Pipeline) retrieveFrom(ctx context.Context, retrievers []Retriever, query Query, meta Meta) ([]Candidate, error) {
	if len(retrievers) == 0 {
		return nil, nil
	}
	workers := p.workers
	if workers > len(retrievers) {
		workers = len(retrievers)
	}
	if workers <= 0 {
		workers = 1
	}
	sets, err := concurrency.Map(ctx, retrievers, workers, func(ctx context.Context, retriever Retriever) ([]Candidate, error) {
		got, rerr := retriever.Retrieve(ctx, query, meta)
		if rerr != nil {
			logger.FromContext(ctx).Warn("retriever failed; skipping", zap.Error(rerr))
			return nil, nil
		}
		return got, nil
	})
	if err != nil {
		return nil, err
	}

	candidates := rrfMerge(sets)
	return candidates, nil
}

func splitRetrieversByCost(retrievers []Retriever) ([]Retriever, []Retriever) {
	cheap := make([]Retriever, 0, len(retrievers))
	expensive := make([]Retriever, 0, len(retrievers))
	for _, retriever := range retrievers {
		if retrievalCost(retriever) == RetrievalCostExpensive {
			expensive = append(expensive, retriever)
			continue
		}
		cheap = append(cheap, retriever)
	}
	return cheap, expensive
}

func retrievalCost(retriever Retriever) RetrievalCost {
	if costAware, ok := retriever.(CostAwareRetriever); ok {
		if cost := costAware.RetrievalCost(); cost != "" {
			return cost
		}
	}
	return RetrievalCostCheap
}

func applyQueryPlan(query Query, plan QueryPlan) Query {
	planned := query
	if plan.RetrievalText != "" {
		planned.Text = plan.RetrievalText
	}
	if plan.EffectiveLimit > 0 {
		planned.Limit = plan.EffectiveLimit
	}
	return planned
}

func timed(timings map[string]int64, name string, fn func() error) error {
	if timings == nil {
		return fn()
	}
	start := time.Now()
	err := fn()
	timings[name] = time.Since(start).Milliseconds()
	return err
}

func newStageTimings(query Query) map[string]int64 {
	if !wantsTimings(query) {
		return nil
	}
	return make(map[string]int64)
}

func wantsTimings(query Query) bool {
	return query.Debug || query.Mode == ModeDebug
}

func debugTrace(query Query, plan QueryPlan, candidates []Candidate, contextAssembled bool, crag CRAGResult, confidence, threshold float64, decision Decision, draft string, criticRan bool) *DebugTrace {
	if !wantsTimings(query) {
		return nil
	}
	topSource := ""
	if len(candidates) > 0 {
		topSource = candidates[0].Source
	}
	return &DebugTrace{
		Mode: query.Mode,
		Plan: DebugPlanTrace{
			Intent:           plan.Meta.Intent,
			Terms:            append([]string(nil), plan.Meta.Terms...),
			RetrievalQueries: len(retrievalQueriesForPlan(query, plan)),
			GraphAnchors:     len(plan.GraphAnchors),
			SkipReasons:      copyStringMap(plan.SkipReasons),
		},
		Retrieval: DebugRetrievalTrace{
			Candidates:       len(candidates),
			TopSource:        topSource,
			ContextAssembled: contextAssembled,
		},
		Quality: DebugQualityTrace{
			CRAGVerdict:    crag.Verdict.String(),
			CRAGConfidence: crag.Confidence,
			CRAGMissing:    crag.Missing,
			Confidence:     confidence,
			Threshold:      threshold,
			Decision:       string(decision),
			DraftGenerated: strings.TrimSpace(draft) != "",
			CriticRan:      criticRan,
		},
	}
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func fastDraft(query Query) bool {
	return query.Mode == ModeFastDraft
}

func shouldRerank(query Query, candidates []Candidate) bool {
	return !(fastDraft(query) && len(candidates) < 2)
}

func shouldRunCRAG(query Query, candidates []Candidate) bool {
	return !(fastDraft(query) && len(candidates) == 0)
}

func shouldRunCritic(query Query, decision Decision, draft string) bool {
	return !fastDraft(query) && decision == DecisionAuto && strings.TrimSpace(draft) != ""
}

func (p *Pipeline) llmAllowed(ctx context.Context, companyID int64, stage string) bool {
	if p.budget == nil {
		return true
	}
	ok, err := p.budget.Acquire(ctx, companyID, stage)
	if err != nil {
		logger.FromContext(ctx).Warn("rag budget check failed; skipping llm stage",
			zap.String("stage", stage), zap.Error(err))
		return false
	}
	return ok
}

func (p *Pipeline) threshold(ctx context.Context, query Query) float64 {
	threshold := p.confidenceThreshold
	if p.thresholdFor != nil {
		if t := p.thresholdFor(ctx, query.CompanyID); t > 0 {
			threshold = t
		}
	}
	return threshold
}

func (p *Pipeline) recordKnowledgeGap(ctx context.Context, query Query, verdict CRAGVerdict) {
	if verdict != VerdictIrrelevant || p.knowledgeGap == nil {
		return
	}
	recordCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	_ = p.knowledgeGap.Record(recordCtx, query.CompanyID, query.Text)
	cancel()
}

func willGenerate(p *Pipeline, decision Decision, verdict CRAGVerdict, candidates []Candidate) bool {
	return p.generator != nil && decision != DecisionEscalate && verdict != VerdictIrrelevant && len(candidates) > 0
}

func (p *Pipeline) generateDraft(ctx context.Context, query Query, verdict CRAGVerdict, decision Decision, candidates []Candidate) (string, error) {
	if p.generator == nil || decision == DecisionEscalate || verdict == VerdictIrrelevant || len(candidates) == 0 {
		return "", nil
	}
	generated, err := p.generator.Generate(ctx, query, candidates)
	if err != nil {
		logger.FromContext(ctx).Warn("rag generator failed", zap.Error(err))
		return "", err
	}
	return generated, nil
}

func (p *Pipeline) critiqueDraft(ctx context.Context, query Query, draft string, candidates []Candidate) CritiqueResult {
	critique := CritiqueResult{Supported: true, Send: true}
	if draft == "" || p.critic == nil {
		return critique
	}
	got, err := p.critic.Critique(ctx, query, draft, candidates)
	if err != nil {
		logger.FromContext(ctx).Warn("rag critic failed; failing closed", zap.Error(err))
		return CritiqueResult{Supported: false, Send: false}
	}
	return got
}

func applyCritiqueDecision(decision Decision, critique CritiqueResult) Decision {
	if !critique.Send && decision == DecisionAuto {
		return DecisionDraft
	}
	return decision
}

func validateQuery(query Query) error {
	if query.CompanyID <= 0 {
		return errors.New("company_id is required")
	}
	if strings.TrimSpace(query.Text) == "" {
		return errors.New("query text is required")
	}
	return nil
}

func topContent(candidates []Candidate) string {
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].Content
}

func computeConfidence(crag CRAGResult, candidates []Candidate) float64 {
	if len(candidates) == 0 {
		return 0
	}
	if crag.Confidence > 0 {
		return clamp01(crag.Confidence)
	}
	switch crag.Verdict {
	case VerdictRelevant:
		return 0.9
	case VerdictPartial:
		return 0.7
	default:
		return 0
	}
}

func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func routeDecision(confidence, threshold float64) Decision {
	switch {
	case confidence >= threshold:
		return DecisionAuto
	case confidence >= 0.60:
		return DecisionDraft
	default:
		return DecisionEscalate
	}
}

func decisionFor(confidence, threshold float64, graded bool) Decision {
	decision := routeDecision(confidence, threshold)
	if decision == DecisionAuto && !graded {
		return DecisionDraft
	}
	return decision
}
