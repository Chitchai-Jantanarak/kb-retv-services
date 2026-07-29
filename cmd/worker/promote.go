package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/my/app/internal/application/workflows/promote"
	"github.com/my/app/internal/domain/ports"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	reviewmysql "github.com/my/app/internal/repositories/review/mysql"
	"github.com/my/app/internal/shared/config"
)

func buildPromoteHandler(cfg config.Config) taskHandler {
	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil || db == nil {
		log.Printf("promote:* disabled: mysql unavailable")
		return unavailable("promote", "mysql not configured")
	}
	workflow, err := promote.New(reviewmysql.NewReviewRepository(db))
	if err != nil {
		log.Printf("promote:* disabled: build workflow: %v", err)
		return unavailable("promote", "workflow build failed")
	}
	log.Println("promote:* handler configured")

	return func(ctx context.Context, job ports.Job) error {
		var payload struct {
			ReviewItemID int64 `json:"review_item_id"`
		}
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("promote: decode payload: %w", err)
		}
		result, err := workflow.Run(ctx, payload.ReviewItemID)
		if err != nil {
			return err
		}
		log.Printf("promote result task=%s company=%d review_item=%d kind=%s ref=%s",
			job.Type, job.CompanyID, result.ReviewItemID, result.Kind, result.PromotionRef)
		return nil
	}
}
