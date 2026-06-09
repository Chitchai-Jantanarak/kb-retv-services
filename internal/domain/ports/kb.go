package ports

import (
	"context"

	"github.com/my/app/internal/domain/kb"
	"github.com/my/app/internal/domain/retrieval"
)

type KnowledgeRepository interface {
	Search(ctx context.Context, query retrieval.Query) ([]kb.Article, error)
}

type KnowledgeChunkRepository interface {
	ChunksByIDs(ctx context.Context, query kb.ChunkQuery) ([]kb.Chunk, error)
}

type KnowledgeParentChunkRepository interface {
	ParentChunksByChildIDs(ctx context.Context, query kb.ChunkQuery) ([]kb.ChunkContext, error)
}
