package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/my/app/internal/application/services/reviewoutbox"
	"github.com/my/app/internal/domain/ports"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	reviewmysql "github.com/my/app/internal/repositories/review/mysql"
	"github.com/my/app/internal/shared/config"
)

func buildOutboxHandler(cfg config.Config) taskHandler {
	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil || db == nil {
		log.Printf("outbox:flush disabled: mysql unavailable")
		return unavailable("outbox:flush", "mysql not configured")
	}
	deliverer, err := reviewoutbox.NewDeliverer(reviewoutbox.DeliveryConfig{
		BaseURL: cfg.Laravel.BaseURL,
		Secret:  cfg.Laravel.WebhookSecret,
		Timeout: time.Duration(cfg.Laravel.Timeout) * time.Second,
	})
	if err != nil {
		log.Printf("outbox:flush disabled: %v", err)
		return unavailable("outbox:flush", err.Error())
	}
	flusher := reviewoutbox.NewFlusher(reviewmysql.NewOutboxWriter(db), deliverer)
	log.Println("outbox:flush handler configured")

	return func(ctx context.Context, job ports.Job) error {
		payload, err := decodeIngestPayload(job.Payload)
		if err != nil {
			return fmt.Errorf("outbox:flush: %w", err)
		}
		result, err := flusher.Run(ctx, reviewoutbox.Options{
			CompanyID: job.CompanyID,
			Limit:     payload.Limit,
		})
		if err != nil {
			return err
		}
		log.Printf("outbox:flush result company=%d scanned=%d sent=%d failed=%d",
			job.CompanyID, result.Scanned, result.Sent, result.Failed)
		return nil
	}
}
