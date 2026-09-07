package main

import (
	"go.uber.org/zap"

	"github.com/my/app/internal/infra/memgraph"
	"github.com/my/app/internal/infra/qdrant"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/transport/http/handlers"
)

type qdrantCollectionNamer struct {
	prefix string
}

func (n qdrantCollectionNamer) CollectionForCompany(companyID int64) (string, error) {
	return qdrant.CollectionForCompany(n.prefix, companyID)
}

func buildKnowledgeStatsHandler(cfg config.Config, log *zap.Logger) *handlers.KnowledgeStatsHandler {
	var graph handlers.KnowledgeGraphQuerier
	if cfg.Memgraph.Enabled {
		store, err := memgraph.NewStore(memgraph.Config{
			URI:      cfg.Memgraph.URI,
			Username: cfg.Memgraph.Username,
			Password: cfg.Memgraph.Password,
		})
		if err != nil {
			log.Warn("knowledge stats: memgraph not configured", zap.Error(err))
		} else {
			graph = store
			log.Info("knowledge stats: memgraph configured")
		}
	}

	var vector handlers.KnowledgeVectorInfo
	var names qdrantCollectionNamer
	if cfg.Qdrant.Enabled {
		vector = qdrant.NewStore(qdrant.Config{URL: cfg.Qdrant.URL, APIKey: cfg.Qdrant.APIKey})
		prefix := cfg.Qdrant.CollectionPrefix
		if prefix == "" {
			prefix = "kb_chunks"
		}
		names = qdrantCollectionNamer{prefix: prefix}
		log.Info("knowledge stats: qdrant configured")
	}

	return handlers.NewKnowledgeStatsHandler(graph, vector, names)
}
