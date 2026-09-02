package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/application/profile"
	"github.com/my/app/internal/application/services/intakeassess"
	"github.com/my/app/internal/application/services/tickets"
	"github.com/my/app/internal/application/workflows/intake"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
	infraasynq "github.com/my/app/internal/infra/asynq"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	channelsmysql "github.com/my/app/internal/repositories/channels/mysql"
	intakemysql "github.com/my/app/internal/repositories/intake/mysql"
	profilemysql "github.com/my/app/internal/repositories/profile/mysql"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

type intakeAssessProducts struct {
	repo profile.Repository
}

func (p intakeAssessProducts) Products(ctx context.Context, companyID int64) ([]string, error) {
	data, err := p.repo.Load(ctx, companyID)
	if err != nil {
		return nil, err
	}
	return data.Products, nil
}

func buildIntakeAssessHandler(cfg config.Config) taskHandler {
	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil || db == nil {
		log.Printf("intake:assess disabled: mysql unavailable: %v", err)
		return unavailable("intake:assess", "mysql not configured")
	}
	router, err := buildTenantQuerier(cfg, db)
	if err != nil {
		log.Printf("intake:assess disabled: tenant router unavailable: %v", err)
		return unavailable("intake:assess", "tenant router not configured")
	}
	acfg := cfg
	if cfg.Intake.AssessTimeoutMs > 0 {
		acfg.LLM.RequestTimeoutSeconds = cfg.Intake.AssessTimeoutMs / 1000
	}
	resolver, err := llmboot.Resolver(acfg, router)
	if err != nil {
		log.Printf("intake:assess disabled: llm resolver unavailable: %v", err)
		return unavailable("intake:assess", "llm resolver not configured")
	}
	registry, err := prompts.NewRegistry()
	if err != nil {
		log.Printf("intake:assess disabled: prompt registry unavailable: %v", err)
		return unavailable("intake:assess", "prompt registry not configured")
	}
	extractor, err := intake.NewExtractor(registry, resolver.ResolveFor,
		intake.WithSpecResolver(intakemysql.NewSpecRepository(router)),
		intake.WithProducts(intakeAssessProducts{repo: profilemysql.New(router)}),
		intake.WithIntentKeywords(intakemysql.NewKeywordRepository(router)),
	)
	if err != nil {
		log.Printf("intake:assess disabled: extractor unavailable: %v", err)
		return unavailable("intake:assess", "extractor not configured")
	}
	logger, lerr := zap.NewProduction()
	if lerr != nil {
		logger = zap.NewNop()
	}
	svc := intake.NewService(extractor, channelsmysql.New(router), logger)

	var ticketEnq *tickets.Enqueuer
	redisURL := cfg.Redis.URL
	if redisURL == "" {
		redisURL = os.Getenv("REDIS_URL")
	}
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	if queue, qerr := infraasynq.NewQueue(infraasynq.Config{RedisURL: redisURL}); qerr != nil {
		log.Printf("intake:assess: ticket auto-promote disabled: %v", qerr)
	} else if enq, eerr := tickets.NewEnqueuer(queue); eerr != nil {
		log.Printf("intake:assess: ticket auto-promote disabled: %v", eerr)
	} else {
		ticketEnq = enq
	}

	timeout := time.Duration(cfg.Intake.AssessTimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	log.Printf("intake:assess handler configured: timeout=%s promote=%t", timeout, ticketEnq != nil)

	return func(ctx context.Context, job ports.Job) error {
		var p intakeassess.Payload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return fmt.Errorf("intake:assess: decode payload: %w", err)
		}
		if job.CompanyID <= 0 || p.ConversationID <= 0 {
			return fmt.Errorf("intake:assess: company=%d conversation=%d invalid", job.CompanyID, p.ConversationID)
		}

		cctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		cctx = ctxkey.WithCompanyID(cctx, job.CompanyID)

		res, err := svc.Assess(cctx, job.CompanyID, p.ConversationID, intake.Signals{
			Sender:          p.Customer,
			Subject:         p.Request.Subject,
			Body:            p.Request.Body,
			ListUnsubscribe: p.ListUnsubscribe,
			AutoSubmitted:   p.AutoSubmitted,
			Precedence:      p.Precedence,
			HasAttachments:  p.HasAttachments,
			ReferencedCase:  p.ReferencedCase,
			ThreadMatched:   p.ThreadMatched,
			Images:          p.Images,
		})
		if err != nil {
			return err
		}

		if ticketEnq != nil && intake.ConfidencePromotable(res.Classification, res.Confidence, res.PromoteThreshold) {
			return ticketEnq.EnqueueTicket(ctx, job.CompanyID, p.ConversationID, p.MessageID, p.Customer, p.Request)
		}
		return nil
	}
}
