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

func gateCandidates(cands []rag.Candidate) []rag.Candidate {
	if len(cands) == 0 || cands[0].Score < knowledgeMinRelevance {
		return nil
	}
	floor := cands[0].Score * knowledgeMinRatio
	out := make([]rag.Candidate, 0, len(cands))
	for _, c := range cands {
		if c.Score >= floor {
			out = append(out, c)
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
	return buildKnowledgeSources(gateCandidates(rag.FTSCandidates(chunks)))
}

func (w *Workflow) rerankedKnowledgeContext(ctx context.Context, companyID int64, query string) (sources []dto.ChatSource, block string, handled bool) {
	coverage := ctxkey.CoverageOrSelf(ctx, companyID)
	chunks, err := w.fts.SearchChunks(ctx, coverage, query, knowledgeCandidateLimit)
	if err != nil {
		logger.FromContext(ctx).Error("chat: knowledge search failed", zap.Int64("company_id", companyID), zap.Error(err))
		return nil, "", true
	}
	reranker := rag.EmbeddingReranker{
		Embedder:      w.knowledgeReranker,
		MinSimilarity: knowledgeMinSimilarity,
		Limit:         knowledgeChunkLimit,
	}
	reranked, err := reranker.Rerank(ctx, rag.Query{CompanyID: companyID, Text: query}, rag.Meta{}, rag.FTSCandidates(chunks))
	if err != nil {
		logger.FromContext(ctx).Warn("chat: knowledge rerank failed, falling back to relevance gate", zap.Int64("company_id", companyID), zap.Error(err))
		return nil, "", false
	}
	sources, block = buildKnowledgeSources(reranked)
	return sources, block, true
}

func buildKnowledgeSources(cands []rag.Candidate) ([]dto.ChatSource, string) {
	var (
		sources []dto.ChatSource
		block   strings.Builder
	)
	for _, c := range cands {
		title := strings.TrimSpace(c.Title)
		snippet := strings.TrimSpace(c.Content)
		if len(snippet) > knowledgeSnippetMaxChars {
			snippet = snippet[:knowledgeSnippetMaxChars]
		}
		if title != "" || snippet != "" {
			sources = append(sources, dto.ChatSource{
				ID:      c.ID,
				Title:   title,
				Snippet: snippet,
				Source:  c.Source,
				Score:   c.Score,
			})
		}
		if snippet != "" {
			fmt.Fprintf(&block, "- %s: %s\n", sanitizeUntrusted(title), sanitizeUntrusted(snippet))
		}
	}
	return sources, untrustedBlock("REFERENCE NOTES", block.String())
}
