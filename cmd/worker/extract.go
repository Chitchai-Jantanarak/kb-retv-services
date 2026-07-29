package main

import (
	"context"
	"fmt"
	"log"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/application/workflows/classify"
	"github.com/my/app/internal/domain/ports"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	classifymysql "github.com/my/app/internal/repositories/classify/mysql"
	reviewmysql "github.com/my/app/internal/repositories/review/mysql"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

func buildExtractHandler(cfg config.Config) taskHandler {
	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil || db == nil {
		log.Printf("extract:batch disabled: mysql unavailable")
		return unavailable("extract:batch", "mysql not configured")
	}
	resolver, err := llmboot.Resolver(cfg, db)
	if err != nil {
		log.Printf("extract:batch disabled: %v", err)
		return unavailable("extract:batch", "resolver failed")
	}
	registry, err := prompts.NewRegistry()
	if err != nil {
		log.Printf("extract:batch disabled: load prompts: %v", err)
		return unavailable("extract:batch", "prompts failed")
	}
	repository := classifymysql.New(db)
	workflow, err := classify.New(classify.Config{
		Registry: registry,
		Source:   repository,
		Lookup:   repository,
		Sink:     repository,
		Resolve:  resolver.ResolveFor,
		Review:   reviewmysql.NewOutboxWriter(db),
		Model:    cfg.LLM.DefaultModel,
	})
	if err != nil {
		log.Printf("extract:batch disabled: build workflow: %v", err)
		return unavailable("extract:batch", "workflow build failed")
	}
	log.Println("extract:batch handler configured")

	return func(ctx context.Context, job ports.Job) error {
		payload, err := decodeIngestPayload(job.Payload)
		if err != nil {
			return fmt.Errorf("extract:batch: %w", err)
		}
		result, err := workflow.Run(ctx, classify.Options{
			CompanyID: job.CompanyID,
			Limit:     payload.Limit,
		})
		if err != nil {
			return err
		}
		log.Printf("extract:batch result company=%d examined=%d classified=%d failed=%d",
			job.CompanyID, result.Examined, result.Classified, result.Failed)
		return nil
	}
}
