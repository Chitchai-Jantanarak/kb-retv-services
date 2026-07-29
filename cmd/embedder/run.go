package main

import (
	"context"
	"log"
	"time"

	infra_mysql "github.com/my/app/internal/infra/mysql"
	"github.com/my/app/internal/shared/config"
)

func run() {
	opts := parseFlags()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if opts.collection == "" {
		opts.collection = cfg.Qdrant.CollectionPrefix
	}

	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	if db == nil {
		log.Fatal("mysql is disabled")
	}
	defer db.Close()

	qdb, closeRouter, err := buildTenantQuerier(cfg, db)
	if err != nil {
		log.Fatalf("build tenant router: %v", err)
	}
	defer closeRouter()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	dispatch(ctx, commandDependencies{cfg: cfg, db: db, qdb: qdb}, opts)
}

func dispatch(ctx context.Context, deps commandDependencies, opts commandOptions) {
	switch opts.task {
	case commandIndexKB:
		runIndexKB(ctx, deps, opts)
	case commandReembed:
		runReembed(ctx, deps, opts)
	case commandBatchSubmit:
		runBatchSubmit(ctx, deps, opts)
	case commandBatchCollect:
		runBatchCollect(ctx, deps, opts)
	case commandBatchRun:
		runBatch(ctx, deps, opts)
	default:
		log.Fatalf("unknown task %q", opts.task)
	}
}
