package toolhandlers

import (
	"context"
	"strconv"
	"strings"

	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/skeleton"
)

const knowledgeSummaryMaxChars = 160

type FTSSource interface {
	SearchChunks(ctx context.Context, coverage []int64, query string, limit int) ([]rag.FTSChunk, error)
}

type Knowledge struct {
	fts FTSSource
}

func NewKnowledge(fts FTSSource) Knowledge {
	return Knowledge{fts: fts}
}

func (h Knowledge) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	limit := q.Tool.Limit
	if limit <= 0 {
		limit = 5
	}
	chunks, err := h.fts.SearchChunks(ctx, q.Coverage, q.Text, limit)
	if err != nil {
		return nil, err
	}

	rows := make([]skeleton.Row, 0, len(chunks))
	for _, chunk := range chunks {
		summary := strings.TrimSpace(chunk.Content)
		if len(summary) > knowledgeSummaryMaxChars {
			summary = summary[:knowledgeSummaryMaxChars]
		}
		rows = append(rows, skeleton.Row{
			"article":    strings.TrimSpace(chunk.Title),
			"summary":    summary,
			"article_id": strconv.FormatInt(chunk.ArticleID, 10),
		})
	}
	return rows, nil
}

type NoopAuditor struct{}

func (NoopAuditor) Log(context.Context, string, skeleton.Actor, string) error {
	return nil
}
