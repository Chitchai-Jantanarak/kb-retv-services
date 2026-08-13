package main

import (
	"context"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/activityaudit"
	"github.com/my/app/internal/application/intent"
	"github.com/my/app/internal/application/profile"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/application/toolaudit"
	"github.com/my/app/internal/application/tools"
	chatwf "github.com/my/app/internal/application/workflows/chat"
	"github.com/my/app/internal/infra/attachments"
	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/infra/memcache"
	"github.com/my/app/internal/infra/tenant"
	transcribegemini "github.com/my/app/internal/infra/transcribe/gemini"
	activitymysql "github.com/my/app/internal/repositories/activity/mysql"
	mysqlai "github.com/my/app/internal/repositories/ai/mysql"
	channelsmysql "github.com/my/app/internal/repositories/channels/mysql"
	chatsessionmysql "github.com/my/app/internal/repositories/chatsession/mysql"
	customermysql "github.com/my/app/internal/repositories/customer/mysql"
	employeemysql "github.com/my/app/internal/repositories/employee/mysql"
	gazetteermysql "github.com/my/app/internal/repositories/gazetteer/mysql"
	profilemysql "github.com/my/app/internal/repositories/profile/mysql"
	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
	"github.com/my/app/internal/shared/perms"
	"github.com/my/app/internal/transport/http/handlers"
)

type chatEndpoints struct {
	chat    *handlers.ChatHandler
	stream  *handlers.ChatStreamHandler
	confirm *handlers.ChatConfirmHandler
}

func buildChatEndpoints(
	cfg config.Config,
	qdb tenant.Querier,
	resolver *llm.CompanyResolver,
	reportsRepo *reportsmysql.Repository,
	log *zap.Logger,
) chatEndpoints {
	var endpoints chatEndpoints

	if resolver == nil {
		log.Warn("chat endpoint not configured: llm resolver unavailable")
		return endpoints
	}

	chatRegistry, err := prompts.NewRegistry()
	if err != nil {
		log.Warn("chat endpoint not configured", zap.Error(err))
		return endpoints
	}

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

	chatOpts, guardErr := appendChatIntelligenceOptions(cfg, qdb, resolver, reportsRepo, ftsSource, chatOpts, log, &endpoints.confirm)
	if guardErr != nil {
		log.Error("chat endpoints disabled: local routing guard unavailable and no provider substitution is permitted",
			zap.Error(guardErr))
		return endpoints
	}

	chatOpts = append(chatOpts,
		chatwf.WithProfile(profile.NewAssembler(profilemysql.New(qdb))),
		chatwf.WithCaseSearch(chatCaseSearch{repo: reportsRepo}),
		chatwf.WithSessions(chatsessionmysql.New(qdb)),
		chatwf.WithTurnRecorder(toolaudit.NewChatTurn(
			activityaudit.NewAIAction(mysqlai.NewActionRecorder(qdb), activitymysql.New(qdb), log),
			agentIDLookup(resolver, log))),
	)

	if geminiKey := llmboot.LLMSettings(cfg).GeminiKey; geminiKey != "" {
		if strings.TrimSpace(cfg.Laravel.BaseURL) != "" {
			transcriber := transcribegemini.New(geminiKey, "", "", 0)
			fetcher := attachments.New(
				cfg.Laravel.BaseURL,
				cfg.Laravel.HostHeader,
				time.Duration(cfg.Laravel.Timeout)*time.Second,
				0,
			)
			chatOpts = append(chatOpts, chatwf.WithTranscription(fetcher, transcriber))
			log.Info("voice transcription configured")
		} else {
			log.Warn("voice transcription not configured: laravel base url is required to fetch audio attachments")
		}
	}

	if workflow, workflowErr := chatwf.New(chatRegistry, resolver.ResolveFor, ftsSource, chatOpts...); workflowErr != nil {
		log.Warn("chat endpoint not configured", zap.Error(workflowErr))
	} else {
		endpoints.chat = handlers.NewChatHandler(workflow)
		endpoints.stream = handlers.NewChatStreamHandler(workflow)
		log.Info("chat endpoint configured")
		log.Info("chat stream endpoint configured")
	}

	return endpoints
}

func appendChatIntelligenceOptions(
	cfg config.Config,
	qdb tenant.Querier,
	resolver *llm.CompanyResolver,
	reportsRepo *reportsmysql.Repository,
	ftsSource *rag.MySQLFTSSource,
	chatOpts []chatwf.Option,
	log *zap.Logger,
	confirm **handlers.ChatConfirmHandler,
) ([]chatwf.Option, error) {
	_, _, sharedEmbedder, err := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg))
	if err != nil {
		log.Warn("chat intent router not configured", zap.Error(err))
		return chatOpts, nil
	}

	guardEmbedder, guardTh, guardReason, guardErr := llmboot.GuardEmbedder(cfg, sharedEmbedder)
	if guardErr != nil {
		return chatOpts, guardErr
	}
	log.Info("chat guard embedder resolved", zap.String("reason", guardReason))
	routerOpts := []intent.Option{}
	if guardTh.HasValues {
		routerOpts = append(routerOpts,
			intent.WithThresholds(guardTh.Floor, guardTh.Accept),
			intent.WithMargin(guardTh.Margin))
	}
	chatRouter, err := intent.New(context.Background(), guardEmbedder, chatwf.NewFTSScorer(ftsSource), routerOpts...)
	if err != nil {
		log.Warn("chat intent router not configured", zap.Error(err))
	} else {
		chatOpts = append(chatOpts, chatwf.WithRouter(chatRouter))
		log.Info("chat intent router configured")
	}

	if guardEmbedder != nil {
		chatOpts = append(chatOpts, chatwf.WithKnowledgeReranker(guardEmbedder))
		log.Info("chat knowledge reranker configured")
	}

	catalog, err := tools.Load(os.DirFS("config/tools"), perms.Set())
	if err != nil {
		log.Warn("tool orchestrator not configured", zap.Error(err))
		return chatOpts, nil
	}

	broker := newToolBroker(toolBrokerDeps{
		FTS:       ftsSource,
		Reports:   reportsRepo,
		Inbound:   channelsmysql.New(qdb),
		Promoter:  reportsRepo,
		Employees: employeemysql.New(qdb),
		Customers: customermysql.New(qdb),
	})
	bound := broker.Bound(catalog)
	selector, err := tools.NewSelector(context.Background(), guardEmbedder, bound,
		tools.WithNameSource(gazetteermysql.NewSource(qdb)))
	if err != nil {
		log.Warn("tool orchestrator not configured", zap.Error(err))
		return chatOpts, nil
	}

	orch := skeleton.New(
		selector,
		bound,
		broker.Handlers(),
		toolaudit.New(
			activityaudit.NewAIAction(mysqlai.NewActionRecorder(qdb), activitymysql.New(qdb), log),
			agentIDLookup(resolver, log),
		),
		skeleton.WithPendingStore(skeleton.NewPendingStore()),
	)
	chatOpts = append(chatOpts, chatwf.WithOrchestrator(orch))
	*confirm = handlers.NewChatConfirmHandler(orch)
	log.Info("tool orchestrator configured",
		zap.Int("bound", len(bound)),
		zap.Strings("unbound", broker.UnboundIDs(catalog)))

	return chatOpts, nil
}
