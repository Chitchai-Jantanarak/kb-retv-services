package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"github.com/my/app/internal/domain/ports"
	infraasynq "github.com/my/app/internal/infra/asynq"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	"github.com/my/app/internal/shared/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	redisURL := cfg.Redis.URL
	if redisURL == "" {
		redisURL = os.Getenv("REDIS_URL")
	}
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		log.Fatalf("parse redis url: %v", err)
	}

	scheduler := asynq.NewScheduler(opt, nil)

	companyIDs, err := scheduledCompanyIDs(context.Background(), cfg)
	if err != nil {
		log.Fatalf("load scheduled companies: %v", err)
	}
	if len(companyIDs) == 0 {
		log.Printf("scheduler has no active companies; set SCHEDULER_COMPANY_IDS or enable mysql")
	}

	registerCompanyTasks(scheduler, companyIDs, "@every 1m", "outbox:flush", 55*time.Second)
	registerCompanyTasks(scheduler, companyIDs, "0 2 * * 0", "cluster:weekly", 6*24*time.Hour)
	registerCompanyTasks(scheduler, companyIDs, "0 */6 * * *", "embedding:refresh", 5*time.Hour+55*time.Minute)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("scheduler starting (redis=%s)", redisURL)
		errCh <- scheduler.Run()
	}()

	select {
	case <-ctx.Done():
		log.Println("scheduler shutting down")
		scheduler.Shutdown()
	case err := <-errCh:
		log.Fatalf("scheduler error: %v", err)
	}
}

func registerCompanyTasks(s *asynq.Scheduler, companyIDs []int64, spec string, taskType string, uniqueFor time.Duration) {
	for _, companyID := range companyIDs {
		task, opts, err := scheduledCompanyTask(taskType, companyID, uniqueFor)
		if err != nil {
			log.Fatalf("build %s for company %d: %v", taskType, companyID, err)
		}
		if _, err := s.Register(spec, task, opts...); err != nil {
			log.Fatalf("register %s for company %d: %v", taskType, companyID, err)
		}
	}
}

func scheduledCompanyTask(taskType string, companyID int64, uniqueFor time.Duration) (*asynq.Task, []asynq.Option, error) {
	if strings.TrimSpace(taskType) == "" {
		return nil, nil, fmt.Errorf("task type is required")
	}
	if companyID <= 0 {
		return nil, nil, fmt.Errorf("company_id must be positive")
	}
	return infraasynq.TaskForJob(ports.Job{
		Type:      taskType,
		CompanyID: companyID,
		UniqueFor: uniqueFor,
	})
}

func scheduledCompanyIDs(ctx context.Context, cfg config.Config) ([]int64, error) {
	if raw := strings.TrimSpace(os.Getenv("SCHEDULER_COMPANY_IDS")); raw != "" {
		return parseCompanyIDs(raw)
	}
	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, nil
	}
	defer func() { _ = db.Close() }()
	return queryActiveCompanyIDs(ctx, db)
}

func parseCompanyIDs(raw string) ([]int64, error) {
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	seen := make(map[int64]struct{}, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid company id %q", part)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func queryActiveCompanyIDs(ctx context.Context, db *sql.DB) ([]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT company_id FROM company_routes ORDER BY company_id ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id > 0 {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}
