package main

import (
	"context"

	"go.uber.org/zap"

	searchwf "github.com/my/app/internal/application/workflows/search"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/memgraph"
	"github.com/my/app/internal/infra/qdrant"
	graphrepo "github.com/my/app/internal/repositories/graph"
	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/transport/http/handlers"
)

type searchCaseSource struct {
	repo *reportsmysql.Repository
}

func (s searchCaseSource) CasesByIDs(ctx context.Context, coverage []int64, ids []int64) ([]searchwf.CaseRecord, error) {
	rows, err := s.repo.CasesByIDs(ctx, coverage, ids)
	if err != nil {
		return nil, err
	}
	out := make([]searchwf.CaseRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, searchwf.CaseRecord{
			ID:        row.ID,
			Code:      row.Code,
			Title:     row.Title,
			Status:    row.Status,
			SymptomID: row.SymptomID,
		})
	}
	return out, nil
}

func buildSearchHandler(
	cfg config.Config,
	reportsRepo *reportsmysql.Repository,
	provider, model string,
	embedder ports.EmbeddingProvider,
	log *zap.Logger,
) *handlers.SearchHandler {
	if !cfg.Qdrant.Enabled {
		log.Info("search endpoint disabled: qdrant not enabled")
		return nil
	}

	if embedder == nil {
		log.Warn("search endpoint not configured: embedding provider unavailable")
		return nil
	}

	collectionPrefix := cfg.Qdrant.CollectionPrefix
	if collectionPrefix == "" {
		collectionPrefix = "kb_chunks"
	}

	wfCfg := searchwf.Config{
		Embedder:         embedder,
		Vector:           qdrant.NewStore(qdrant.Config{URL: cfg.Qdrant.URL, APIKey: cfg.Qdrant.APIKey}),
		Cases:            searchCaseSource{repo: reportsRepo},
		CollectionPrefix: collectionPrefix,
	}

	if cfg.Memgraph.Enabled {
		store, err := memgraph.NewStore(memgraph.Config{
			URI:      cfg.Memgraph.URI,
			Username: cfg.Memgraph.Username,
			Password: cfg.Memgraph.Password,
		})
		if err != nil {
			log.Warn("search graph half disabled: memgraph open failed", zap.Error(err))
		} else {
			wfCfg.Graph = graphrepo.NewKBGraph(store)
		}
	} else {
		log.Warn("search graph half disabled: MEMGRAPH_ENABLED is false, kb results will be empty")
	}

	workflow, err := searchwf.New(wfCfg)
	if err != nil {
		log.Warn("search endpoint not configured", zap.Error(err))
		return nil
	}

	log.Info("search endpoint configured",
		zap.String("provider", provider),
		zap.String("model", model),
		zap.String("collection_prefix", collectionPrefix),
		zap.Bool("graph", wfCfg.Graph != nil))

	return handlers.NewSearchHandler(workflow)
}
