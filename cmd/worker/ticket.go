package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/services/tickets"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/config"
)

const classifyAfterTicketLimit = 25

func buildTicketCreateHandler(cfg config.Config, classifyQueue ports.Queue) taskHandler {
	deliverer, err := tickets.NewDeliverer(tickets.DeliveryConfig{
		BaseURL: cfg.Laravel.BaseURL,
		Path:    cfg.Laravel.TicketPath,
		Secret:  cfg.Laravel.WebhookSecret,
		Timeout: time.Duration(cfg.Laravel.Timeout) * time.Second,
	})
	if err != nil {
		log.Printf("ticket:create disabled: %v", err)
		return unavailable("ticket:create", err.Error())
	}
	log.Printf("ticket:create handler configured: base=%s path=%s classify_trigger=%t",
		cfg.Laravel.BaseURL, cfg.Laravel.TicketPath, classifyQueue != nil)

	return func(ctx context.Context, job ports.Job) error {
		var payload tickets.Payload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("ticket:create: decode payload: %w", err)
		}
		if payload.CompanyID <= 0 {
			payload.CompanyID = job.CompanyID
		}
		if payload.CompanyID != job.CompanyID {
			return fmt.Errorf("ticket:create: payload company %d != job company %d", payload.CompanyID, job.CompanyID)
		}

		body, err := buildTicketBody(payload)
		if err != nil {
			return err
		}
		if err := deliverer.Deliver(ctx, body); err != nil {
			return err
		}
		enqueueClassify(ctx, classifyQueue, payload.CompanyID, job.TraceID)
		return nil
	}
}

func enqueueClassify(ctx context.Context, queue ports.Queue, companyID int64, traceID string) {
	if queue == nil {
		return
	}
	payload, err := json.Marshal(dto.IngestReportPayload{Limit: classifyAfterTicketLimit})
	if err != nil {
		log.Printf("ticket:create: marshal classify payload company=%d err=%v", companyID, err)
		return
	}
	if err := queue.Enqueue(ctx, ports.Job{
		Type:      TaskExtractBatch,
		CompanyID: companyID,
		TraceID:   traceID,
		Payload:   payload,
	}); err != nil {
		log.Printf("ticket:create: enqueue extract:batch company=%d err=%v", companyID, err)
	}
}

func buildTicketBody(payload tickets.Payload) ([]byte, error) {
	envelope := map[string]any{
		"company_id":      payload.CompanyID,
		"channel":         payload.Channel,
		"conversation_id": payload.ConversationID,
		"message_id":      payload.MessageID,
		"subject":         payload.Subject,
		"body":            payload.Body,
		"sender_external": payload.SenderExternal,
		"sender_name":     payload.SenderName,
		"idempotency_key": payload.IdempotencyKey(),
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("ticket:create: marshal envelope: %w", err)
	}
	return body, nil
}
