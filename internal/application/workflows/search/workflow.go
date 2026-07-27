package search

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
)

const snippetMaxRunes = 320

type FTSSource interface {
	SearchChunks(ctx context.Context, coverage []int64, query string, limit int) ([]rag.FTSChunk, error)
}

type Workflow struct {
	fts FTSSource
}

func NewWorkflow(fts FTSSource) *Workflow {
	return &Workflow{fts: fts}
}

func (w *Workflow) Run(ctx context.Context, req dto.SearchRequest) (dto.SearchResponse, error) {
	if err := req.Validate(); err != nil {
		return dto.SearchResponse{}, err
	}
	companyID, ok := ctxkey.CompanyID(ctx)
	if !ok || companyID <= 0 {
		return dto.SearchResponse{}, apperr.New(apperr.CodeUnauthorized, "company scope is required")
	}

	resp := dto.SearchResponse{Query: req.Query, Results: []dto.SearchResult{}}
	if w.fts == nil {
		return resp, nil
	}

	chunks, err := w.fts.SearchChunks(ctx, []int64{companyID}, req.Query, req.Limit)
	if err != nil {
		return dto.SearchResponse{}, err
	}
	results := make([]dto.SearchResult, 0, len(chunks))
	for _, chunk := range chunks {
		results = append(results, dto.SearchResult{
			Type:      dto.SearchTypeKB,
			ID:        fmt.Sprintf("chunk:%d", chunk.ChunkID),
			ArticleID: strconv.FormatInt(chunk.ArticleID, 10),
			Title:     strings.TrimSpace(chunk.Title),
			Snippet:   truncateSnippet(chunk.Content),
			Score:     chunk.Relevance,
		})
	}
	resp.Results = results
	return resp, nil
}

func truncateSnippet(content string) string {
	trimmed := strings.TrimSpace(content)
	runes := []rune(trimmed)
	if len(runes) <= snippetMaxRunes {
		return trimmed
	}
	return string(runes[:snippetMaxRunes]) + "..."
}
