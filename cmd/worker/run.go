package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"

	"github.com/my/app/internal/domain/ports"
	infraasynq "github.com/my/app/internal/infra/asynq"
	"github.com/my/app/internal/shared/config"
)

func run() {
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

	redisOptions, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		log.Fatalf("parse redis url: %v", err)
	}

	server := asynq.NewServer(redisOptions, asynq.Config{
		Concurrency: 10,
		Queues: map[string]int{
			"critical": 6,
			"default":  3,
			"low":      1,
		},
	})

	var classifyQueue ports.Queue
	queue, err := infraasynq.NewQueue(infraasynq.Config{RedisURL: redisURL})
	if err != nil {
		log.Printf("classify trigger disabled: %v", err)
	} else {
		classifyQueue = queue
	}

	mux := buildTaskMux(cfg, classifyQueue)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("worker starting (redis=%s)", redisURL)
		errCh <- server.Run(mux)
	}()

	select {
	case <-ctx.Done():
		log.Println("worker shutting down")
		server.Shutdown()
	case err := <-errCh:
		log.Fatalf("worker error: %v", err)
	}
}
