package main

import (
	"database/sql"

	"go.uber.org/zap"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/application/services/intakeassess"
	instructionswf "github.com/my/app/internal/application/workflows/instructions"
	promotewf "github.com/my/app/internal/application/workflows/promote"
	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/infra/tenant"
	mysqlai "github.com/my/app/internal/repositories/ai/mysql"
	channelsmysql "github.com/my/app/internal/repositories/channels/mysql"
	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
	reviewmysql "github.com/my/app/internal/repositories/review/mysql"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
	"github.com/my/app/internal/transport/http/handlers"
	appmiddleware "github.com/my/app/internal/transport/http/middleware"
)

type apiHandlers struct {
	reports      *handlers.ReportsHandler
	inbound      *handlers.InboundHandler
	feedback     *handlers.FeedbackHandler
	review       *handlers.ReviewHandler
	chat         *handlers.ChatHandler
	chatStream   *handlers.ChatStreamHandler
	chatConfirm  *handlers.ChatConfirmHandler
	search       *handlers.SearchHandler
	intake       *handlers.IntakeAssessHandler
	instructions *handlers.InstructionsEnhanceHandler
	aiUsage      *handlers.AIUsageHandler
	aiModels     *handlers.AIModelsHandler
	chatModels   *handlers.ChatModelsHandler
	knowledge    *handlers.KnowledgeStatsHandler
}

func buildAPIHandlers(
	cfg config.Config,
	central *sql.DB,
	qdb tenant.Querier,
	resolver *llm.CompanyResolver,
	features appmiddleware.FeatureReader,
	log *zap.Logger,
) apiHandlers {
	var endpoints apiHandlers
	if qdb == nil {
		return endpoints
	}

	reportsRepo := reportsmysql.New(qdb)
	embProvider, embModel, embedder, embErr := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg))
	if embErr != nil {
		log.Warn("embedding provider not configured", zap.Error(embErr))
	}
	chat := buildChatEndpoints(cfg, qdb, resolver, reportsRepo, embedder, log)
	endpoints.chat = chat.chat
	endpoints.chatStream = chat.stream
	endpoints.chatConfirm = chat.confirm

	endpoints.reports = handlers.NewReportsHandler(reportsRepo)
	log.Info("reports endpoints configured")

	endpoints.aiUsage = handlers.NewAIUsageHandler(mysqlai.NewRequestLogRepo(qdb))
	endpoints.aiModels = handlers.NewAIModelsHandler(llm.NewModelCatalog(mysqlai.NewRoutingRepository(qdb, cfg.LLM.ProviderConfigKey), cfg.LLM.LocalURL))
	if resolver != nil {
		endpoints.chatModels = handlers.NewChatModelsHandler(resolver)
	}
	log.Info("ai usage endpoint configured")

	endpoints.knowledge = buildKnowledgeStatsHandler(cfg, log)
	log.Info("knowledge stats endpoints configured")

	endpoints.search = buildSearchHandler(cfg, reportsRepo, embProvider, embModel, embedder, log)

	if resolver != nil {
		if instrRegistry, instrErr := prompts.NewRegistry(); instrErr != nil {
			log.Warn("instructions enhance endpoint not configured", zap.Error(instrErr))
		} else if enhancer, enhancerErr := instructionswf.New(instrRegistry, resolver.ForTask("instructions_enhance")); enhancerErr != nil {
			log.Warn("instructions enhance endpoint not configured", zap.Error(enhancerErr))
		} else {
			endpoints.instructions = handlers.NewInstructionsEnhanceHandler(enhancer)
			log.Info("instructions enhance endpoint configured")
		}
	}

	if queue := buildAssessEnqueuer(cfg, log); queue != nil {
		manual, manualErr := intakeassess.NewManual(channelsmysql.New(qdb), queue)
		if manualErr != nil {
			log.Warn("manual intake assessment not configured", zap.Error(manualErr))
		} else {
			endpoints.intake = handlers.NewIntakeAssessHandler(manual)
			log.Info("manual intake assessment endpoint configured")
		}
	}

	inbound, err := buildInboundHandler(cfg, central, qdb, resolver, features, log)
	if err != nil {
		log.Warn("inbound webhooks not configured", zap.Error(err))
	} else {
		endpoints.inbound = inbound
		log.Info("inbound webhook endpoints configured", zap.Strings("channels", inboundChannels()))
	}

	endpoints.feedback = handlers.NewFeedbackHandler(mysqlai.NewFeedbackRecorder(qdb))
	log.Info("reply feedback endpoint configured")

	reviewRepo := reviewmysql.NewReviewRepository(qdb)
	promoteWorkflow, err := promotewf.New(reviewRepo)
	if err != nil {
		log.Warn("promote workflow not configured", zap.Error(err))
	} else {
		endpoints.review = handlers.NewReviewHandler(promoteWorkflow, reviewRepo)
		log.Info("admin review-queue endpoints configured")
	}

	return endpoints
}
