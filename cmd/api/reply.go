package main

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/my/app/internal/ai/budget"
	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/activityaudit"
	"github.com/my/app/internal/application/workflows/reply"
	"github.com/my/app/internal/domain/kb"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/infra/memcache"
	"github.com/my/app/internal/infra/memgraph"
	"github.com/my/app/internal/infra/qdrant"
	"github.com/my/app/internal/infra/tenant"
	activitymysql "github.com/my/app/internal/repositories/activity/mysql"
	mysqlai "github.com/my/app/internal/repositories/ai/mysql"
	"github.com/my/app/internal/repositories/graph"
	"github.com/my/app/internal/repositories/kb/memory"
	mysqlkb "github.com/my/app/internal/repositories/kb/mysql"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

func buildKnowledge(db tenant.Querier) ports.KnowledgeRepository {
	if db != nil {
		return mysqlkb.NewKnowledgeRepository(db)
	}
	return memory.NewKnowledgeRepository([]kb.Article{
		{
			ID:        "kb-printer-offline",
			CompanyID: 1,
			Title:     "Printer offline checklist",
			Summary:   "Please check printer power, network connectivity, and queue status before escalating.",
		},
	})
}

func buildWorkflowOptions(cfg config.Config, db tenant.Querier, log *zap.Logger, resolver *llm.CompanyResolver) []reply.Option {
	opts := make([]reply.Option, 0, 8)
	if cfg.Reply.DefaultMode == "fast_draft" {
		opts = append(opts, reply.WithDefaultMode(rag.ModeFastDraft))
		log.Info("server-defaulted reply mode configured", zap.String("mode", rag.ModeFastDraft))
	}
	opts = append(opts, reply.WithBudget(budget.New(budget.Config{
		Cache:                    memcache.New(4096),
		MaxCallsPerRequest:       cfg.Budget.MaxLLMCallsPerRequest,
		MaxCallsPerCompanyWindow: cfg.Budget.MaxLLMCallsPerCompanyWindow,
		Window:                   time.Duration(cfg.Budget.WindowSeconds) * time.Second,
	})))
	if db == nil {
		return opts
	}
	opts = append(opts, reply.WithKnowledgeGap(mysqlkb.NewKnowledgeGapStore(db)))
	log.Info("knowledge gap recorder configured")

	if resolver == nil {
		return appendRetrievers(opts, cfg, db, log)
	}

	registry, regErr := prompts.NewRegistry()
	if regErr != nil {
		log.Fatal("load prompt registry", zap.Error(regErr))
	}
	opts = append(opts, llmStages(registry, resolver, log)...)
	opts = append(
		opts,
		reply.WithThresholdFor(thresholdLookup(resolver, log)),
		reply.WithActionRecorder(
			activityaudit.NewAIAction(mysqlai.NewActionRecorder(db), activitymysql.New(db), log),
			agentIDLookup(resolver, log),
		),
	)
	log.Info("per-company confidence threshold lookup configured")
	log.Info("ai_actions recorder configured")

	return appendRetrievers(opts, cfg, db, log)
}

func llmStages(registry *prompts.Registry, resolver *llm.CompanyResolver, log *zap.Logger) []reply.Option {
	opts := make([]reply.Option, 0, 4)
	if crag, err := rag.NewLLMCRAG(registry, resolver.ResolveFor); err != nil {
		log.Warn("crag not configured", zap.Error(err))
	} else {
		opts = append(opts, reply.WithCRAG(crag))
		log.Info("llm-backed CRAG configured")
	}
	if reranker, err := rag.NewLLMReranker(registry, resolver.ResolveFor, rag.LexicalReranker{}); err != nil {
		log.Warn("llm reranker not configured", zap.Error(err))
	} else {
		opts = append(opts, reply.WithReranker(reranker))
		log.Info("llm-backed reranker configured")
	}
	if generator, err := rag.NewLLMGenerator(rag.LLMGeneratorConfig{
		Registry: registry,
		Resolve:  resolver.ResolveFor,
	}); err != nil {
		log.Warn("llm generator not configured", zap.Error(err))
	} else {
		opts = append(opts, reply.WithGenerator(generator))
		log.Info("llm-backed generator configured")
	}
	if critic, err := rag.NewLLMCritic(registry, resolver.ResolveFor); err != nil {
		log.Warn("llm critic not configured", zap.Error(err))
	} else {
		opts = append(opts, reply.WithCritic(critic))
		log.Info("self-rag critic configured")
	}
	return opts
}

func appendRetrievers(opts []reply.Option, cfg config.Config, db tenant.Querier, log *zap.Logger) []reply.Option {
	if r, err := rag.NewFTSRetriever(rag.NewMySQLFTSSource(db)); err != nil {
		log.Warn("fts retriever not configured", zap.Error(err))
	} else {
		opts = append(opts, reply.WithRetriever(r))
		log.Info("mysql FTS retriever configured")
	}

	if cfg.Memgraph.Enabled {
		store, err := memgraph.NewStore(memgraph.Config{
			URI:      cfg.Memgraph.URI,
			Username: cfg.Memgraph.Username,
			Password: cfg.Memgraph.Password,
		})
		if err != nil {
			log.Warn("graph retriever not configured", zap.Error(err))
		} else {
			opts = append(opts, reply.WithGraphRetriever(rag.NewGraphAnchorRetriever(graph.NewKBGraph(store))))
			log.Info("graph anchor retriever configured")
		}
	}

	if !cfg.Qdrant.Enabled {
		return opts
	}
	provider, model, embedder, err := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg))
	if err != nil {
		log.Fatal("build embedder", zap.Error(err))
	}
	collectionPrefix := cfg.Qdrant.CollectionPrefix
	if collectionPrefix == "" {
		collectionPrefix = "kb_chunks"
	}
	opts = append(opts, reply.WithRetriever(rag.NewVectorRetriever(rag.VectorRetrieverConfig{
		Embedder:         embedder,
		Vector:           qdrant.NewStore(qdrant.Config{URL: cfg.Qdrant.URL, APIKey: cfg.Qdrant.APIKey}),
		Chunks:           mysqlkb.NewKnowledgeRepository(db),
		CollectionPrefix: collectionPrefix,
	})))
	log.Info("vector retriever configured",
		zap.String("provider", provider),
		zap.String("model", model),
		zap.String("collection_prefix", collectionPrefix))
	return opts
}

func thresholdLookup(resolver *llm.CompanyResolver, log *zap.Logger) rag.ThresholdForCompany {
	return func(ctx context.Context, companyID int64) float64 {
		agent, err := resolver.AgentFor(ctx, companyID)
		if err != nil {
			log.Warn("agent lookup failed; using pipeline default",
				zap.Int64("company_id", companyID), zap.Error(err))
			return 0
		}
		return agent.ConfidenceThreshold
	}
}

func agentIDLookup(resolver *llm.CompanyResolver, log *zap.Logger) reply.AgentIDLookup {
	return func(ctx context.Context, companyID int64) (int64, bool) {
		agent, err := resolver.AgentFor(ctx, companyID)
		if err != nil {
			log.Warn("agent id lookup failed",
				zap.Int64("company_id", companyID), zap.Error(err))
			return 0, false
		}
		if agent.ID <= 0 {
			return 0, false
		}
		return agent.ID, true
	}
}
