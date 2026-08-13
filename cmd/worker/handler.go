package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hibiken/asynq"

	"github.com/my/app/internal/domain/ports"
	infraasynq "github.com/my/app/internal/infra/asynq"
)

type taskHandler func(ctx context.Context, job ports.Job) error

func makeHandler(fn taskHandler) asynq.HandlerFunc {
	return func(ctx context.Context, task *asynq.Task) error {
		if err := ctx.Err(); err != nil {
			log.Printf("task %s skipped: context already done: %v", task.Type(), err)
			return err
		}

		job, err := infraasynq.ExtractJob(task)
		if err != nil {
			log.Printf("task %s decode error: %v", task.Type(), err)
			return err
		}

		start := time.Now()
		log.Printf("task start type=%s company=%d trace=%s", job.Type, job.CompanyID, job.TraceID)

		handlerErr := fn(ctx, job)

		elapsed := time.Since(start)
		if handlerErr != nil {
			log.Printf("task error type=%s company=%d elapsed=%s err=%v", job.Type, job.CompanyID, elapsed, handlerErr)
		} else {
			log.Printf("task done type=%s company=%d elapsed=%s", job.Type, job.CompanyID, elapsed)
		}
		return handlerErr
	}
}

func unavailable(name, reason string) taskHandler {
	err := fmt.Errorf("%s unavailable: %s", name, reason)
	return func(_ context.Context, _ ports.Job) error { return err }
}
