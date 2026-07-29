package rag

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/my/app/internal/shared/logger"
)

func (p *Pipeline) generate(ctx context.Context, state *pipelineState) error {
	if willGenerate(p, state.decision, state.verdict, state.candidates) &&
		p.llmAllowed(ctx, state.originalQuery.CompanyID, "generate") {
		if err := timed(state.timings, "generate", func() error {
			state.draft, state.generationErr = p.generateDraft(
				ctx,
				state.query,
				state.verdict,
				state.decision,
				state.candidates,
			)
			return nil
		}); err != nil {
			return err
		}
	}

	state.critique = CritiqueResult{Supported: true, Send: true}
	if shouldRunCritic(state.query, state.decision, state.draft) {
		if p.llmAllowed(ctx, state.originalQuery.CompanyID, "critic") {
			if err := timed(state.timings, "critique", func() error {
				state.criticRan = true
				state.critique = p.critiqueDraft(ctx, state.query, state.draft, state.candidates)
				return nil
			}); err != nil {
				return err
			}
			state.decision = applyCritiqueDecision(state.decision, state.critique)
		} else {
			state.decision = DecisionDraft
		}
	}
	if state.decision == DecisionAuto && strings.TrimSpace(state.draft) == "" {
		state.decision = DecisionEscalate
	}

	if state.decision == DecisionEscalate {
		reason, retryAfter := classifyEscalation(state.generationErr)
		state.reason = reason
		state.retryAfterMS = retryAfter.Milliseconds()
	}
	return nil
}

func shouldRunCritic(query Query, decision Decision, draft string) bool {
	return !fastDraft(query) && decision == DecisionAuto && strings.TrimSpace(draft) != ""
}

func willGenerate(p *Pipeline, decision Decision, verdict CRAGVerdict, candidates []Candidate) bool {
	return p.generator != nil &&
		decision != DecisionEscalate &&
		verdict != VerdictIrrelevant &&
		len(candidates) > 0
}

func (p *Pipeline) generateDraft(
	ctx context.Context,
	query Query,
	verdict CRAGVerdict,
	decision Decision,
	candidates []Candidate,
) (string, error) {
	if p.generator == nil ||
		decision == DecisionEscalate ||
		verdict == VerdictIrrelevant ||
		len(candidates) == 0 {
		return "", nil
	}
	generated, err := p.generator.Generate(ctx, query, candidates)
	if err != nil {
		logger.FromContext(ctx).Warn("rag generator failed", zap.Error(err))
		return "", err
	}
	return generated, nil
}

func (p *Pipeline) critiqueDraft(
	ctx context.Context,
	query Query,
	draft string,
	candidates []Candidate,
) CritiqueResult {
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
