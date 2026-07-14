package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"go.uber.org/zap"

	"github.com/my/app/internal/ai/budget"
	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/intent"
	"github.com/my/app/internal/application/profile"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/application/toolaudit"
	"github.com/my/app/internal/application/toolbroker"
	"github.com/my/app/internal/application/tools"
	chatwf "github.com/my/app/internal/application/workflows/chat"
	"github.com/my/app/internal/application/workflows/intake"
	"github.com/my/app/internal/application/workflows/omnichannel"
	promotewf "github.com/my/app/internal/application/workflows/promote"
	"github.com/my/app/internal/application/workflows/reply"
	"github.com/my/app/internal/domain/kb"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/infra/memcache"
	"github.com/my/app/internal/infra/memgraph"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	"github.com/my/app/internal/infra/qdrant"
	"github.com/my/app/internal/infra/tenant"
	mysqlai "github.com/my/app/internal/repositories/ai/mysql"
	channelsmysql "github.com/my/app/internal/repositories/channels/mysql"
	customermysql "github.com/my/app/internal/repositories/customer/mysql"
	employeemysql "github.com/my/app/internal/repositories/employee/mysql"
	"github.com/my/app/internal/repositories/graph"
	intakemysql "github.com/my/app/internal/repositories/intake/mysql"
	"github.com/my/app/internal/repositories/kb/memory"
	mysqlkb "github.com/my/app/internal/repositories/kb/mysql"
	profilemysql "github.com/my/app/internal/repositories/profile/mysql"
	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
	reviewmysql "github.com/my/app/internal/repositories/review/mysql"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
	"github.com/my/app/internal/shared/logger"
	"github.com/my/app/internal/shared/perms"
	"github.com/my/app/internal/transport/http/handlers"
	appmiddleware "github.com/my/app/internal/transport/http/middleware"
	"github.com/my/app/internal/transport/http/routes"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic("load config: " + err.Error())
	}

	if err := logger.Init(logger.Config{
		Level:      cfg.Logger.Level,
		Format:     cfg.Logger.Format,
		OutputPath: cfg.Logger.OutputPath,
	}); err != nil {
		panic("init logger: " + err.Error())
	}
	defer func() { _ = logger.Get().Sync() }()

	log := logger.Get()

	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil {
		log.Fatal("open mysql", zap.Error(err))
	}
	var qdb tenant.Querier
	if db != nil {
		defer func() { _ = db.Close() }()
		log.Info("mysql configured")

		pool, perr := tenant.NewPool(db, func(name string) (*sql.DB, error) {
			return infra_mysql.OpenForDB(cfg.MySQL, name)
		})
		if perr != nil {
			log.Fatal("build tenant pool", zap.Error(perr))
		}
		defer pool.Close()
		qdb = pool.Router()
		log.Info("tenant DB-swarm router configured")
	}

	var resolver *llm.CompanyResolver
	if qdb != nil {
		if r, rerr := llmboot.Resolver(cfg, qdb); rerr != nil {
			log.Warn("llm resolver not configured", zap.Error(rerr))
		} else {
			resolver = r
			log.Info("llm company resolver configured",
				zap.String("default_vendor", cfg.LLM.DefaultVendor),
				zap.String("default_model", cfg.LLM.DefaultModel))
		}
	}

	knowledge := buildKnowledge(qdb)
	workflow := reply.NewWorkflow(knowledge, buildWorkflowOptions(cfg, qdb, log, resolver)...)

	e := echo.New()
	var reportsHandler *handlers.ReportsHandler
	var inboundHandler *handlers.InboundHandler
	var feedbackHandler *handlers.FeedbackHandler
	var reviewHandler *handlers.ReviewHandler
	var chatHandler *handlers.ChatHandler
	if qdb != nil {
		reportsRepo := reportsmysql.New(qdb)
		if resolver == nil {
			log.Warn("chat endpoint not configured: llm resolver unavailable")
		} else if chatRegistry, regErr := prompts.NewRegistry(); regErr != nil {
			log.Warn("chat endpoint not configured", zap.Error(regErr))
		} else {
			chatOpts := make([]chatwf.Option, 0, 1)
			if cfg.Chat.CacheTTLSeconds > 0 {
				chatOpts = append(chatOpts, chatwf.WithCache(
					memcache.New(cfg.Chat.CacheMaxEntries),
					time.Duration(cfg.Chat.CacheTTLSeconds)*time.Second,
				))
				log.Info("chat response cache configured",
					zap.Int("ttl_seconds", cfg.Chat.CacheTTLSeconds),
					zap.Int("max_entries", cfg.Chat.CacheMaxEntries))
			}
			ftsSource := rag.NewMySQLFTSSource(qdb)
			if _, _, sharedEmbedder, eerr := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg)); eerr != nil {
				log.Warn("chat intent router not configured", zap.Error(eerr))
			} else {
				guardEmbedder, guardTh, guardReason := llmboot.GuardEmbedder(cfg, sharedEmbedder)
				log.Info("chat guard embedder resolved", zap.String("reason", guardReason))
				routerOpts := []intent.Option{}
				if guardTh.HasValues {
					routerOpts = append(routerOpts,
						intent.WithThresholds(guardTh.Floor, guardTh.Accept),
						intent.WithMargin(guardTh.Margin))
				}
				if chatRouter, rerr := intent.New(context.Background(), guardEmbedder, chatwf.NewFTSScorer(ftsSource), routerOpts...); rerr != nil {
					log.Warn("chat intent router not configured", zap.Error(rerr))
				} else {
					chatOpts = append(chatOpts, chatwf.WithRouter(chatRouter))
					log.Info("chat intent router configured")
				}
				if catalog, terr := tools.Load(os.DirFS("config/tools"), perms.Set()); terr != nil {
					log.Warn("tool orchestrator not configured", zap.Error(terr))
				} else {
					broker := toolbroker.New(toolbroker.Deps{FTS: ftsSource, Reports: reportsRepo, Inbound: channelsmysql.New(qdb), Promoter: reportsRepo, Employees: employeemysql.New(qdb), Customers: customermysql.New(qdb)})
					bound := broker.Bound(catalog)
					if selector, serr := tools.NewSelector(context.Background(), sharedEmbedder, bound); serr != nil {
						log.Warn("tool orchestrator not configured", zap.Error(serr))
					} else {
						orch := skeleton.New(selector, bound, broker.Handlers(), toolaudit.New(mysqlai.NewActionRecorder(qdb), agentIDLookup(resolver, log)))
						chatOpts = append(chatOpts, chatwf.WithOrchestrator(orch))
						log.Info("tool orchestrator configured",
							zap.Int("bound", len(bound)),
							zap.Strings("unbound", broker.UnboundIDs(catalog)))
					}
				}
			}
			chatOpts = append(chatOpts,
				chatwf.WithProfile(profile.NewAssembler(profilemysql.New(qdb))),
				chatwf.WithCaseSearch(chatCaseSearch{repo: reportsRepo}),
			)
			if cw, cerr := chatwf.New(chatRegistry, resolver.ResolveFor, ftsSource, chatOpts...); cerr != nil {
				log.Warn("chat endpoint not configured", zap.Error(cerr))
			} else {
				chatHandler = handlers.NewChatHandler(cw)
				log.Info("chat endpoint configured")
			}
		}
		reportsHandler = handlers.NewReportsHandler(reportsRepo)
		log.Info("reports endpoints configured")

		if h, err := buildInboundHandler(cfg, db, qdb, resolver, log); err != nil {
			log.Warn("inbound webhooks not configured", zap.Error(err))
		} else {
			inboundHandler = h
			log.Info("inbound webhook endpoints configured", zap.Strings("channels", inboundChannels()))
		}

		feedbackHandler = handlers.NewFeedbackHandler(mysqlai.NewFeedbackRecorder(qdb))
		log.Info("reply feedback endpoint configured")

		reviewRepo := reviewmysql.NewReviewRepository(qdb)
		if pw, err := promotewf.New(reviewRepo); err != nil {
			log.Warn("promote workflow not configured", zap.Error(err))
		} else {
			reviewHandler = handlers.NewReviewHandler(pw, reviewRepo)
			log.Info("admin review-queue endpoints configured")
		}
	}
	keyMaterial := serviceJWTKeyMaterial(cfg, log)
	if cfg.App.IsProduction() && keyMaterial == "" {
		log.Fatal("service JWT key material is required in production; set the Laravel public key path or PEM")
	}
	routes.Register(e, handlers.NewReplyHandler(workflow), routes.Options{
		Log:            log,
		SwaggerEnabled: cfg.Swagger.EnabledFor(cfg.App),
		JWTSecret:      keyMaterial,
		RequireAuth:    cfg.App.IsProduction(),
		Reports:        reportsHandler,
		Inbound:        inboundHandler,
		Feedback:       feedbackHandler,
		Review:         reviewHandler,
		Chat:           chatHandler,
		Budget: appmiddleware.BudgetPolicy{
			Fallback: time.Duration(cfg.Server.RequestBudgetMs) * time.Millisecond,
			Headroom: time.Duration(cfg.Server.DeadlineHeadroomMs) * time.Millisecond,
			Min:      time.Duration(cfg.Server.DeadlineMinMs) * time.Millisecond,
			Max:      time.Duration(cfg.Server.DeadlineMaxMs) * time.Millisecond,
		},
	})

	log.Info("starting server", zap.String("port", cfg.Server.Port))
	if err := e.Start(":" + cfg.Server.Port); err != nil {
		log.Fatal("start api", zap.Error(err))
	}
}

func serviceJWTKeyMaterial(cfg config.Config, log *zap.Logger) string {
	if path := strings.TrimSpace(cfg.Laravel.JWTPublicKeyPath); path != "" {
		raw, err := os.ReadFile(path) //nolint:gosec
		if err != nil {
			log.Fatal("read service JWT public key", zap.String("path", path), zap.Error(err))
		}
		return string(raw)
	}
	if strings.TrimSpace(cfg.Laravel.JWKSURL) != "" {
		log.Warn("service JWT JWKS URL configured but not yet fetched; falling back to inline public key material")
	}
	return cfg.Laravel.JWTSecret
}

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
	opts = append(opts, reply.WithBudget(budget.New(budget.Config{
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
		reply.WithActionRecorder(mysqlai.NewActionRecorder(db), agentIDLookup(resolver, log)),
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

func buildInboundHandler(cfg config.Config, central, router tenant.Querier, resolver *llm.CompanyResolver, log *zap.Logger) (*handlers.InboundHandler, error) {
	if cfg.App.IsProduction() && strings.TrimSpace(cfg.Laravel.WebhookSecret) == "" {
		return nil, errors.New("inbound webhook secret is required in production")
	}
	centralRepo := channelsmysql.New(central)
	siloRepo := channelsmysql.New(router)
	wfCfg := omnichannel.Config{
		Accounts:      centralRepo,
		Conversations: siloRepo,
		Messages:      siloRepo,
	}
	if assessor := buildIntakeAssessor(router, siloRepo, resolver, log); assessor != nil {
		wfCfg.Completeness = assessor
		log.Info("email intake completeness check configured")
	}

	wf, err := omnichannel.New(wfCfg)
	if err != nil {
		return nil, err
	}
	registry, err := omnichannel.NewNormalizerRegistry(
		omnichannel.LineNormalizer{},
		omnichannel.EmailNormalizer{},
	)
	if err != nil {
		return nil, err
	}
	return handlers.NewInboundHandler(wf, registry, handlers.WithInboundWebhookSecret(cfg.Laravel.WebhookSecret)), nil
}

func buildIntakeAssessor(router tenant.Querier, sink intake.Sink, resolver *llm.CompanyResolver, log *zap.Logger) omnichannel.CompletenessAssessor {
	if router == nil || resolver == nil {
		log.Warn("email intake completeness check not configured: llm resolver unavailable")
		return nil
	}
	registry, err := prompts.NewRegistry()
	if err != nil {
		log.Warn("email intake completeness check not configured", zap.Error(err))
		return nil
	}
	extractor, err := intake.NewExtractor(registry, resolver.ResolveFor,
		intake.WithSpecResolver(intakemysql.NewSpecRepository(router)),
		intake.WithProducts(intakeProducts{repo: profilemysql.New(router)}),
	)
	if err != nil {
		log.Warn("email intake completeness check not configured", zap.Error(err))
		return nil
	}
	return intakeAssessor{svc: intake.NewService(extractor, sink, log)}
}

func inboundChannels() []string {
	return []string{omnichannel.ChannelLine, omnichannel.ChannelEmail}
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
