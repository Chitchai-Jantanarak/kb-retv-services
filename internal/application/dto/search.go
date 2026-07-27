package dto

import (
	apperr "github.com/my/app/internal/shared/errors"
)

const (
	SearchTypeKB = "kb"
)

const (
	searchDefaultLimit  = 10
	searchMinLimit      = 1
	searchMaxLimit      = 50
	searchMaxQueryChars = 4000
)

type SearchRequest struct {
	Query string   `json:"query"`
	Types []string `json:"types,omitempty"`
	Limit int      `json:"limit,omitempty"`
}

func (r *SearchRequest) Normalize() {
	r.Query = normalizeText(r.Query)
	if r.Limit == 0 {
		r.Limit = searchDefaultLimit
	}
	if len(r.Types) == 0 {
		r.Types = []string{SearchTypeKB}
	}
}

func (r *SearchRequest) Validate() error {
	r.Normalize()
	if r.Query == "" {
		return apperr.New(apperr.CodeInvalidInput, "query is required")
	}
	if len(r.Query) > searchMaxQueryChars {
		return apperr.New(apperr.CodeInvalidInput, "query is too large")
	}
	if r.Limit < searchMinLimit || r.Limit > searchMaxLimit {
		return apperr.New(apperr.CodeInvalidInput, "limit must be between 1 and 50")
	}
	for _, t := range r.Types {
		if t != SearchTypeKB {
			return apperr.New(apperr.CodeInvalidInput, "unsupported type: "+t)
		}
	}
	return nil
}

type SearchResult struct {
	Type      string  `json:"type"`
	ID        string  `json:"id"`
	ArticleID string  `json:"article_id,omitempty"`
	Title     string  `json:"title,omitempty"`
	Snippet   string  `json:"snippet,omitempty"`
	Score     float64 `json:"score,omitempty"`
}

type SearchResponse struct {
	Query   string         `json:"query"`
	Results []SearchResult `json:"results"`
}
