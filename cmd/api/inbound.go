package main

import (
	"errors"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/application/services/mediastore"
	"github.com/my/app/internal/application/services/tickets"
	"github.com/my/app/internal/application/workflows/intake"
	"github.com/my/app/internal/application/workflows/omnichannel"
	infraasynq "github.com/my/app/internal/infra/asynq"
	lineinfra "github.com/my/app/internal/infra/line"
	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/infra/tenant"
	activitymysql "github.com/my/app/internal/repositories/activity/mysql"
	channelsmysql "github.com/my/app/internal/repositories/channels/mysql"
	intakemysql "github.com/my/app/internal/repositories/intake/mysql"
	profilemysql "github.com/my/app/internal/repositories/profile/mysql"
	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/transport/http/handlers"
)

func buildInboundHandler(cfg config.Config, central, router tenant.Querier, resolver *llm.CompanyResolver, log *zap.Logger) (*handlers.InboundHandler, error) {
	if cfg.App.IsProduction() && strings.TrimSpace(cfg.Laravel.WebhookSecret) == "" {
		return nil, errors.New("inbound webhook secret is required in production")
	}
	centralRepo := channelsmysql.New(central)
	siloRepo := channelsmysql.New(router)
	reportsRepo := reportsmysql.New(router)
	wfCfg := omnichannel.Config{
		Accounts:       centralRepo,
		Conversations:  siloRepo,
		Messages:       siloRepo,
		CaseLookup:     caseLookupAdapter{repo: reportsRepo},
		CustomerLookup: reportsRepo,
		AppKey:         cfg.App.Key,
		Backfill:       siloRepo,
		Activity:       activitymysql.New(router),
		Log:            log,
	}
	if assessor := buildIntakeAssessor(router, siloRepo, resolver, log); assessor != nil {
		wfCfg.Completeness = assessor
		log.Info("email intake completeness check configured")
	}
	if promoter := buildMediaPromoter(cfg, log); promoter != nil {
		wfCfg.MediaPromoter = promoter
	}
	if enqueuer := buildTicketEnqueuer(cfg, log); enqueuer != nil {
		wfCfg.Tickets = enqueuer
		log.Info("ticket auto-enqueue configured")
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
	opts := []handlers.InboundOption{handlers.WithInboundWebhookSecret(cfg.Laravel.WebhookSecret)}
	if strings.TrimSpace(cfg.Line.ChannelSecret) != "" {
		opts = append(opts,
			handlers.WithChannelVerifier(omnichannel.ChannelLine, handlers.LineSignatureVerifier(cfg.Line.ChannelSecret)),
			handlers.WithChannelSignatureHeader(omnichannel.ChannelLine, "X-Line-Signature"),
		)
		log.Info("line webhook signature verification enabled")
	}
	return handlers.NewInboundHandler(wf, registry, opts...), nil
}

func buildMediaPromoter(cfg config.Config, log *zap.Logger) *mediastore.Promoter {
	if strings.TrimSpace(cfg.Laravel.WebhookSecret) == "" ||
		strings.TrimSpace(cfg.Laravel.BaseURL) == "" {
		log.Warn("media promotion disabled: laravel delivery not configured",
			zap.Bool("laravel_webhook_secret", strings.TrimSpace(cfg.Laravel.WebhookSecret) != ""),
			zap.Bool("laravel_base_url", strings.TrimSpace(cfg.Laravel.BaseURL) != ""))
		return nil
	}
	deliverer, err := mediastore.NewDeliverer(mediastore.DeliveryConfig{
		BaseURL: cfg.Laravel.BaseURL,
		Secret:  cfg.Laravel.WebhookSecret,
		Timeout: time.Duration(cfg.Laravel.Timeout) * time.Second,
	})
	if err != nil {
		log.Warn("media promotion not configured", zap.Error(err))
		return nil
	}

	var contentClient mediastore.ContentFetcher
	if token := strings.TrimSpace(cfg.Line.ChannelAccessToken); token != "" {
		contentClient = lineinfra.New("", token, 0)
		log.Info("media promotion configured", zap.Bool("line_fetch", true))
	} else {
		log.Info("media promotion configured for inline payloads only", zap.Bool("line_fetch", false))
	}
	return mediastore.NewPromoter(contentClient, deliverer)
}

func buildTicketEnqueuer(cfg config.Config, log *zap.Logger) *tickets.Enqueuer {
	redisURL := cfg.Redis.URL
	if redisURL == "" {
		redisURL = os.Getenv("REDIS_URL")
	}
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	queue, err := infraasynq.NewQueue(infraasynq.Config{RedisURL: redisURL})
	if err != nil {
		log.Warn("ticket auto-enqueue not configured", zap.Error(err))
		return nil
	}
	enqueuer, err := tickets.NewEnqueuer(queue)
	if err != nil {
		log.Warn("ticket auto-enqueue not configured", zap.Error(err))
		return nil
	}
	return enqueuer
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
