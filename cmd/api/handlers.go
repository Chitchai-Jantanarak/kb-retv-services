package main

import (
	"database/sql"

	"go.uber.org/zap"

	promotewf "github.com/my/app/internal/application/workflows/promote"
	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/infra/tenant"
	mysqlai "github.com/my/app/internal/repositories/ai/mysql"
	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
	reviewmysql "github.com/my/app/internal/repositories/review/mysql"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/transport/http/handlers"
)

type apiHandlers struct {
	reports     *handlers.ReportsHandler
	inbound     *handlers.InboundHandler
	feedback    *handlers.FeedbackHandler
	review      *handlers.ReviewHandler
	chat        *handlers.ChatHandler
	chatStream  *handlers.ChatStreamHandler
	chatConfirm *handlers.ChatConfirmHandler
}

func buildAPIHandlers(
	cfg config.Config,
	central *sql.DB,
	qdb tenant.Querier,
	resolver *llm.CompanyResolver,
	log *zap.Logger,
) apiHandlers {
	var endpoints apiHandlers
	if qdb == nil {
		return endpoints
	}

	reportsRepo := reportsmysql.New(qdb)
	chat := buildChatEndpoints(cfg, qdb, resolver, reportsRepo, log)
	endpoints.chat = chat.chat
	endpoints.chatStream = chat.stream
	endpoints.chatConfirm = chat.confirm

	endpoints.reports = handlers.NewReportsHandler(reportsRepo)
	log.Info("reports endpoints configured")

	inbound, err := buildInboundHandler(cfg, central, qdb, resolver, log)
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
