package dto

import (
	"slices"

	apperr "github.com/my/app/internal/shared/errors"
)

const (
	SearchTypeReport = "report"
	SearchTypeKB     = "kb"
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
		r.Types = []string{SearchTypeReport, SearchTypeKB}
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
		if t != SearchTypeReport && t != SearchTypeKB {
			return apperr.New(apperr.CodeInvalidInput, "unsupported type: "+t)
		}
	}
	return nil
}

func (r *SearchRequest) Wants(searchType string) bool {
	return slices.Contains(r.Types, searchType)
}

type SearchResult struct {
	Type      string  `json:"type"`
	ID        string  `json:"id"`
	ReportID  int64   `json:"report_id,omitempty"`
	ArticleID string  `json:"article_id,omitempty"`
	Code      string  `json:"code,omitempty"`
	Title     string  `json:"title,omitempty"`
	Status    string  `json:"status,omitempty"`
	Source    string  `json:"source"`
	Score     float64 `json:"score"`
}

type SearchResponse struct {
	Query        string         `json:"query"`
	Results      []SearchResult `json:"results"`
	GraphUsed    bool           `json:"graph_used"`
	GraphAnchors int            `json:"graph_anchors"`
	VectorHits   int            `json:"vector_hits"`
}
