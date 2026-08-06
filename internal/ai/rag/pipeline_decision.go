package rag

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/my/app/internal/shared/logger"
)

func (p *Pipeline) evaluate(ctx context.Context, state *pipelineState) error {
	state.cragResult = CRAGResult{Verdict: VerdictIrrelevant}
	if shouldRunCRAG(state.query, state.candidates) &&
		p.llmAllowed(ctx, state.originalQuery.CompanyID, "crag") {
		if err := timed(state.timings, "crag", func() error {
			var err error
			state.cragResult, err = p.crag.Grade(
				ctx,
				state.query.Text,
				evidenceContent(state.candidates),
			)
			return err
		}); err != nil {
			return err
		}
	}

	state.verdict = state.cragResult.Verdict
	state.confidence = computeConfidence(state.cragResult, state.candidates)
	state.threshold = p.threshold(ctx, state.query)
	state.decision = decisionFor(state.confidence, state.threshold, state.cragResult.Graded)
	p.recordKnowledgeGap(ctx, state.originalQuery, state.verdict)
	return nil
}

func (p *Pipeline) finalize(state *pipelineState) Result {
	return Result{
		Meta:           state.plan.Meta,
		Candidates:     state.candidates,
		Context:        p.compressor.Compress(state.candidates),
		Confidence:     state.confidence,
		Decision:       state.decision,
		Reason:         state.reason,
		RetryAfterMs:   state.retryAfterMS,
		Draft:          state.draft,
		CRAG:           state.cragResult,
		Critique:       state.critique,
		StageTimingsMS: state.timings,
		DebugTrace: debugTrace(
			state.query,
			state.plan,
			state.candidates,
			state.contextAssembled,
			state.cragResult,
			state.confidence,
			state.threshold,
			state.decision,
			state.draft,
			state.criticRan,
			state.graphError,
		),
	}
}

func (p *Pipeline) cacheResult(ctx context.Context, key string, result Result) error {
	cachedResult := result
	cachedResult.StageTimingsMS = nil
	cachedResult.DebugTrace = nil
	return setCached(ctx, p.cache.store, key, cachedResult, p.cache.ttl)
}

func shouldRunCRAG(query Query, candidates []Candidate) bool {
	return !fastDraft(query) || len(candidates) > 0
}

func (p *Pipeline) llmAllowed(ctx context.Context, companyID int64, stage string) bool {
	if p.budget == nil {
		return true
	}
	ok, err := p.budget.Acquire(ctx, companyID, stage)
	if err != nil {
		logger.FromContext(ctx).Warn(
			"rag budget check failed; skipping llm stage",
			zap.String("stage", stage),
			zap.Error(err),
		)
		return false
	}
	return ok
}

func (p *Pipeline) threshold(ctx context.Context, query Query) float64 {
	threshold := p.confidenceThreshold
	if p.thresholdFor != nil {
		if configured := p.thresholdFor(ctx, query.CompanyID); configured > 0 {
			threshold = configured
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

func evidenceContent(candidates []Candidate) string {
	if len(candidates) == 0 {
		return ""
	}
	var builder strings.Builder
	for i, candidate := range candidates {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(candidate.Content)
	}
	return builder.String()
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
