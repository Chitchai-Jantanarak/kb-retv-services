package main

import (
	"context"
	"log"
	"time"

	"github.com/my/app/internal/application/workflows/graphsync"
	"github.com/my/app/internal/infra/memgraph"
	"github.com/my/app/internal/infra/tenant"
	"github.com/my/app/internal/repositories/graph"
	mysqlkb "github.com/my/app/internal/repositories/kb/mysql"
	"github.com/my/app/internal/shared/config"
)

func buildGraphsyncWorkflow(cfg config.Config, db tenant.Querier) *graphsync.Workflow {
	if !cfg.Memgraph.Enabled || db == nil {
		return nil
	}
	store, err := memgraph.NewStore(memgraph.Config{
		URI:      cfg.Memgraph.URI,
		Username: cfg.Memgraph.Username,
		Password: cfg.Memgraph.Password,
	})
	if err != nil {
		log.Printf("graphsync disabled: memgraph open: %v", err)
		return nil
	}
	workflow, err := graphsync.New(graphsync.Config{
		Source: mysqlkb.NewArticleSource(db),
		Graph:  graph.NewKBGraph(store),
	})
	if err != nil {
		log.Printf("graphsync disabled: build workflow: %v", err)
		return nil
	}
	return workflow
}

func runGraphsync(ctx context.Context, workflow *graphsync.Workflow, companyID int64, since time.Time, articleCount int) {
	result, err := workflow.Run(ctx, graphsync.Options{
		CompanyID: companyID,
		Since:     since,
		Limit:     articleCount + 16,
	})
	if err != nil {
		log.Printf("graphsync error company=%d err=%v", companyID, err)
		return
	}
	log.Printf("graphsync result company=%d articles=%d edges=%d skipped=%d",
		companyID, result.Articles, result.Edges, result.Skipped)
}
