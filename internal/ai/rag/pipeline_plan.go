package rag

import (
	"context"
	"errors"
	"strings"

	"github.com/my/app/internal/shared/ctxkey"
)

func (p *Pipeline) prepare(ctx context.Context, query Query) (context.Context, *pipelineState, error) {
	if err := validateQuery(query); err != nil {
		return ctx, nil, err
	}
	if err := ctx.Err(); err != nil {
		return ctx, nil, err
	}

	ctx = ctxkey.WithCompanyID(ctx, query.CompanyID)
	if p.budget != nil {
		ctx = p.budget.Begin(ctx)
	}

	state := &pipelineState{
		originalQuery: query,
		timings:       newStageTimings(query),
		reason:        ReasonNone,
	}
	if err := timed(state.timings, "extract", func() error {
		var err error
		state.plan, err = p.planner.Plan(ctx, query)
		return err
	}); err != nil {
		return ctx, nil, err
	}

	state.query = applyQueryPlan(query, state.plan)
	state.cacheKey = cacheKeyForPlan(query, state.plan)
	return ctx, state, nil
}

func (p *Pipeline) cachedResult(ctx context.Context, state *pipelineState) (Result, bool, error) {
	if wantsTimings(state.originalQuery) {
		return Result{}, false, nil
	}

	cached, ok, err := getCached(ctx, p.cache.store, state.cacheKey)
	if err != nil || !ok {
		return cached, ok, err
	}
	cached.Decision = decisionFor(cached.Confidence, p.threshold(ctx, state.query), cached.CRAG.Graded)
	if cached.Decision == DecisionAuto && strings.TrimSpace(cached.Draft) == "" {
		cached.Decision = DecisionEscalate
	}
	return cached, true, nil
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

func validateQuery(query Query) error {
	if query.CompanyID <= 0 {
		return errors.New("company_id is required")
	}
	if strings.TrimSpace(query.Text) == "" {
		return errors.New("query text is required")
	}
	return nil
}
