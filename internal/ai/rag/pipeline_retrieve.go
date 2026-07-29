package rag

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"

	"github.com/my/app/internal/shared/logger"
	"github.com/my/app/pkg/concurrency"
)

func (p *Pipeline) retrieve(ctx context.Context, state *pipelineState) error {
	if err := timed(state.timings, "retrieve", func() error {
		var err error
		state.candidates, err = p.retrieveCandidates(ctx, state.query, state.plan, state.timings)
		return err
	}); err != nil {
		return err
	}

	if p.graphRetriever != nil && len(state.plan.GraphAnchors) > 0 {
		if err := timed(state.timings, "retrieve_graph", func() error {
			graphCandidates, err := p.graphRetriever.RetrieveGraph(ctx, state.query, state.plan)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return err
				}
				logger.FromContext(ctx).Warn("graph retriever failed; skipping", zap.Error(err))
				return nil
			}
			state.candidates = rrfMerge([][]Candidate{graphCandidates, state.candidates})
			return nil
		}); err != nil {
			return err
		}
	}

	if p.contextAssembler != nil && len(state.candidates) > 0 {
		if err := timed(state.timings, "assemble_context", func() error {
			var err error
			state.candidates, err = p.contextAssembler.Assemble(ctx, state.query, state.candidates)
			state.contextAssembled = true
			return err
		}); err != nil {
			return err
		}
	}

	if shouldRerank(state.query, state.candidates) &&
		p.llmAllowed(ctx, state.originalQuery.CompanyID, "rerank") {
		if err := timed(state.timings, "rerank", func() error {
			var err error
			state.candidates, err = p.reranker.Rerank(ctx, state.query, state.plan.Meta, state.candidates)
			return err
		}); err != nil {
			return err
		}
	}
	if state.query.Limit > 0 && len(state.candidates) > state.query.Limit {
		state.candidates = state.candidates[:state.query.Limit]
	}
	return nil
}

func (p *Pipeline) retrieveCandidates(
	ctx context.Context,
	query Query,
	plan QueryPlan,
	timings map[string]int64,
) ([]Candidate, error) {
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
		candidates, handled, err := p.retrieveFastDraft(ctx, query, meta, timings)
		if err != nil || handled {
			return candidates, err
		}
	}
	return p.retrieveFrom(ctx, p.retrievers, query, meta)
}

func (p *Pipeline) retrieveFastDraft(
	ctx context.Context,
	query Query,
	meta Meta,
	timings map[string]int64,
) ([]Candidate, bool, error) {
	cheap, expensive := splitRetrieversByCost(p.retrievers)
	if len(cheap) == 0 || len(expensive) == 0 {
		return nil, false, nil
	}

	var candidates []Candidate
	if err := timed(timings, "retrieve_cheap", func() error {
		var err error
		candidates, err = p.retrieveFrom(ctx, cheap, query, meta)
		return err
	}); err != nil {
		return nil, true, err
	}
	if len(candidates) > 0 {
		return candidates, true, nil
	}
	if err := timed(timings, "retrieve_expensive", func() error {
		var err error
		candidates, err = p.retrieveFrom(ctx, expensive, query, meta)
		return err
	}); err != nil {
		return nil, true, err
	}
	return candidates, true, nil
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

func (p *Pipeline) retrieveFrom(
	ctx context.Context,
	retrievers []Retriever,
	query Query,
	meta Meta,
) ([]Candidate, error) {
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
	sets, err := concurrency.Map(
		ctx,
		retrievers,
		workers,
		func(ctx context.Context, retriever Retriever) ([]Candidate, error) {
			got, rerr := retriever.Retrieve(ctx, query, meta)
			if rerr != nil {
				logger.FromContext(ctx).Warn("retriever failed; skipping", zap.Error(rerr))
				return nil, nil
			}
			return got, nil
		},
	)
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

func fastDraft(query Query) bool {
	return query.Mode == ModeFastDraft
}

func shouldRerank(query Query, _ []Candidate) bool {
	return !fastDraft(query)
}
