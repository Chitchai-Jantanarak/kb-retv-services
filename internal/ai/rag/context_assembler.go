package rag

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"

	"github.com/my/app/internal/domain/kb"
	"github.com/my/app/internal/shared/logger"
)

type ParentChunkSource interface {
	ParentChunksByChildIDs(ctx context.Context, query kb.ChunkQuery) ([]kb.ChunkContext, error)
}

type ParentChildContextAssembler struct {
	source ParentChunkSource
}

func NewContextAssembler(source ParentChunkSource) *ParentChildContextAssembler {
	return &ParentChildContextAssembler{source: source}
}

func (a *ParentChildContextAssembler) Assemble(ctx context.Context, query Query, candidates []Candidate) ([]Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a.source == nil || len(candidates) == 0 {
		return candidates, nil
	}

	ids := candidateChunkIDs(candidates)
	if len(ids) == 0 {
		return candidates, nil
	}

	rows, err := a.source.ParentChunksByChildIDs(ctx, kb.ChunkQuery{
		IDs:       ids,
		CompanyID: query.CompanyID,
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		logger.FromContext(ctx).Warn("parent context expansion failed; keeping child chunks", zap.Error(err))
		return candidates, nil
	}
	if len(rows) == 0 {
		return candidates, nil
	}

	byChild := make(map[string]kb.ChunkContext, len(rows))
	for _, row := range rows {
		if row.ChildID == "" {
			continue
		}
		byChild[row.ChildID] = row
	}

	out := make([]Candidate, 0, len(candidates))
	seen := make(map[string]int, len(candidates))
	for _, candidate := range candidates {
		assembled := candidate
		if chunkID := candidateChunkID(candidate); chunkID != "" {
			if row, ok := byChild[chunkID]; ok {
				assembled = candidateWithParentContext(candidate, row)
			}
		}

		key := assembled.ID
		if key == "" {
			key = assembled.Source + ":" + assembled.Title + ":" + assembled.Content
		}
		if index, ok := seen[key]; ok {
			if assembled.Score > out[index].Score {
				out[index].Score = assembled.Score
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, assembled)
	}
	return out, nil
}

func candidateChunkIDs(candidates []Candidate) []string {
	out := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		id := candidateChunkID(candidate)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func candidateChunkID(candidate Candidate) string {
	if candidate.ChunkID != "" {
		return candidate.ChunkID
	}
	if strings.HasPrefix(candidate.ID, "chunk:") {
		return strings.TrimPrefix(candidate.ID, "chunk:")
	}
	before, after, ok := strings.Cut(candidate.ID, ":")
	if ok && before != "" && after != "" && !strings.Contains(after, ":") {
		return after
	}
	return ""
}

func candidateWithParentContext(candidate Candidate, row kb.ChunkContext) Candidate {
	out := candidate
	out.ArticleID = firstNonEmpty(row.ArticleID, candidate.ArticleID)
	out.ParentChunkID = row.ParentID
	if out.ChunkID == "" {
		out.ChunkID = row.ChildID
	}
	if row.Title != "" {
		out.Title = row.Title
	}
	if row.Content != "" {
		out.Content = row.Content
	}
	if row.ArticleID != "" && row.ParentID != "" {
		out.ID = row.ArticleID + ":" + row.ParentID
	} else if row.ParentID != "" {
		out.ID = "parent:" + row.ParentID
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
