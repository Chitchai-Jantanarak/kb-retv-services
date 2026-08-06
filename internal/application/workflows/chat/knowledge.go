package chat

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/shared/ctxkey"
	"github.com/my/app/internal/shared/logger"
)

func gateChunks(chunks []rag.FTSChunk) []rag.FTSChunk {
	if len(chunks) == 0 || chunks[0].Relevance < knowledgeMinRelevance {
		return nil
	}
	floor := chunks[0].Relevance * knowledgeMinRatio
	out := make([]rag.FTSChunk, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk.Relevance >= floor {
			out = append(out, chunk)
		}
	}
	return out
}

func (w *Workflow) knowledgeContext(ctx context.Context, companyID int64, query string) ([]dto.ChatSource, string) {
	if w.fts == nil {
		return nil, ""
	}
	chunks, err := w.fts.SearchChunks(ctx, ctxkey.CoverageOrSelf(ctx, companyID), query, knowledgeChunkLimit)
	if err != nil {
		logger.FromContext(ctx).Error("chat: knowledge search failed", zap.Int64("company_id", companyID), zap.Error(err))
		return nil, ""
	}
	chunks = gateChunks(chunks)

	var (
		sources []dto.ChatSource
		block   strings.Builder
	)
	for _, chunk := range chunks {
		title, snippet := rag.SnippetFromChunk(chunk, true, knowledgeSnippetMaxChars)
		if title != "" || snippet != "" {
			sources = append(sources, dto.ChatSource{
				ID:      fmt.Sprintf("chunk:%d", chunk.ChunkID),
				Title:   title,
				Snippet: snippet,
				Source:  "fts",
				Score:   chunk.Relevance,
			})
		}
		if snippet != "" {
			fmt.Fprintf(&block, "- %s: %s\n", title, snippet)
		}
	}
	return sources, block.String()
}
