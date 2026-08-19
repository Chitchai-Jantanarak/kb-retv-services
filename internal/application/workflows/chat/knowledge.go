package chat

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/shared/ctxkey"
	"github.com/my/app/internal/shared/logger"
	"github.com/my/app/internal/shared/vec"
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
	if w.knowledgeReranker != nil {
		if sources, block, handled := w.rerankedKnowledgeContext(ctx, companyID, query); handled {
			return sources, block
		}
	}
	chunks, err := w.fts.SearchChunks(ctx, ctxkey.CoverageOrSelf(ctx, companyID), query, knowledgeChunkLimit)
	if err != nil {
		logger.FromContext(ctx).Error("chat: knowledge search failed", zap.Int64("company_id", companyID), zap.Error(err))
		return nil, ""
	}
	return buildKnowledgeSources(gateChunks(chunks))
}

func (w *Workflow) rerankedKnowledgeContext(ctx context.Context, companyID int64, query string) (sources []dto.ChatSource, block string, handled bool) {
	coverage := ctxkey.CoverageOrSelf(ctx, companyID)
	chunks, err := w.fts.SearchChunks(ctx, coverage, query, knowledgeCandidateLimit)
	if err != nil {
		logger.FromContext(ctx).Error("chat: knowledge search failed", zap.Int64("company_id", companyID), zap.Error(err))
		return nil, "", true
	}
	reranked, err := w.rerankChunks(ctx, query, chunks)
	if err != nil {
		logger.FromContext(ctx).Warn("chat: knowledge rerank failed, falling back to relevance gate", zap.Int64("company_id", companyID), zap.Error(err))
		return nil, "", false
	}
	sources, block = buildKnowledgeSources(reranked)
	return sources, block, true
}

func (w *Workflow) rerankChunks(ctx context.Context, query string, chunks []rag.FTSChunk) ([]rag.FTSChunk, error) {
	if len(chunks) == 0 {
		return nil, nil
	}
	texts := make([]string, 0, len(chunks)+1)
	texts = append(texts, query)
	for _, chunk := range chunks {
		texts = append(texts, chunk.Content)
	}
	vectors, err := w.knowledgeReranker.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(texts) {
		return nil, fmt.Errorf("chat: knowledge rerank: embed returned %d vectors, want %d", len(vectors), len(texts))
	}

	queryVec := vec.Normalize(vectors[0])
	type scoredChunk struct {
		chunk rag.FTSChunk
		score float64
	}
	scored := make([]scoredChunk, 0, len(chunks))
	for i, chunk := range chunks {
		candVec := vec.Normalize(vectors[i+1])
		score := vec.Dot(queryVec, candVec)
		if score < knowledgeMinSimilarity {
			continue
		}
		chunk.Relevance = score
		scored = append(scored, scoredChunk{chunk: chunk, score: score})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if len(scored) > knowledgeChunkLimit {
		scored = scored[:knowledgeChunkLimit]
	}

	out := make([]rag.FTSChunk, len(scored))
	for i, s := range scored {
		out[i] = s.chunk
	}
	return out, nil
}

func buildKnowledgeSources(chunks []rag.FTSChunk) ([]dto.ChatSource, string) {
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
			fmt.Fprintf(&block, "- %s: %s\n", sanitizeUntrusted(title), sanitizeUntrusted(snippet))
		}
	}
	return sources, untrustedBlock("REFERENCE NOTES", block.String())
}
