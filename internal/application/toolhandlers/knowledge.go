package toolhandlers

import (
	"context"
	"strconv"

	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/shared/htmltext"
)

const (
	knowledgeSummaryMaxChars  = 160
	knowledgeToolMinRelevance = 5.0
)

type Knowledge struct {
	fts rag.FTSSource
}

func NewKnowledge(fts rag.FTSSource) Knowledge {
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
		if chunk.Relevance < knowledgeToolMinRelevance {
			continue
		}
		title, content := rag.SnippetFromChunk(chunk, true, 0)
		summary := htmltext.RedactSecrets(content)
		if len(summary) > knowledgeSummaryMaxChars {
			summary = summary[:knowledgeSummaryMaxChars]
		}
		rows = append(rows, skeleton.Row{
			"article":    title,
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
