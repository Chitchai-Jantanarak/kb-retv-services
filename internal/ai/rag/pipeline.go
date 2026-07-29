package rag

import (
	"context"
	"time"

	"github.com/my/app/internal/domain/ports"
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
	ctx, state, err := p.prepare(ctx, query)
	if err != nil {
		return Result{}, err
	}

	if cached, ok, err := p.cachedResult(ctx, state); err != nil {
		return Result{}, err
	} else if ok {
		return cached, nil
	}

	if err := p.retrieve(ctx, state); err != nil {
		return Result{}, err
	}
	if err := p.evaluate(ctx, state); err != nil {
		return Result{}, err
	}
	if err := p.generate(ctx, state); err != nil {
		return Result{}, err
	}

	result := p.finalize(state)
	if err := p.cacheResult(ctx, state.cacheKey, result); err != nil {
		return Result{}, err
	}
	return result, nil
}
