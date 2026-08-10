package search

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/qdrant"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
)

const (
	defaultCollectionPrefix = "kb_chunks"
	vectorOverfetch         = 5
	graphLimit              = 8
)

type CaseRecord struct {
	ID        int64
	Code      string
	Title     string
	Status    string
	SymptomID int64
}

type CaseSource interface {
	CasesByIDs(ctx context.Context, coverage []int64, ids []int64) ([]CaseRecord, error)
}

type GraphSource interface {
	ArticlesForSymptom(ctx context.Context, companyID int64, coverage []int64, symptomID int64, limit int) ([]map[string]any, error)
}

type Config struct {
	Embedder         ports.EmbeddingProvider
	Vector           ports.VectorStore
	Cases            CaseSource
	Graph            GraphSource
	CollectionPrefix string
}

type Workflow struct {
	embedder ports.EmbeddingProvider
	vector   ports.VectorStore
	cases    CaseSource
	graph    GraphSource
	prefix   string
}

func New(cfg Config) (*Workflow, error) {
	if cfg.Embedder == nil {
		return nil, fmt.Errorf("search: embedder is required")
	}
	if cfg.Vector == nil {
		return nil, fmt.Errorf("search: vector store is required")
	}
	if cfg.Cases == nil {
		return nil, fmt.Errorf("search: case source is required")
	}
	prefix := cfg.CollectionPrefix
	if prefix == "" {
		prefix = defaultCollectionPrefix
	}
	return &Workflow{
		embedder: cfg.Embedder,
		vector:   cfg.Vector,
		cases:    cfg.Cases,
		graph:    cfg.Graph,
		prefix:   prefix,
	}, nil
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
	coverage := ctxkey.CoverageOrSelf(ctx, companyID)
	if len(coverage) == 0 {
		return resp, nil
	}

	vectors, err := w.embedder.Embed(ctx, []string{req.Query})
	if err != nil {
		return dto.SearchResponse{}, err
	}
	if len(vectors) != 1 {
		return dto.SearchResponse{}, fmt.Errorf("search: query embedding count %d does not match input count 1", len(vectors))
	}

	scores, order, err := w.vectorReportScores(ctx, coverage, vectors[0], req.Limit*vectorOverfetch)
	if err != nil {
		return dto.SearchResponse{}, err
	}
	resp.VectorHits = len(order)
	if len(order) == 0 {
		return resp, nil
	}

	rows, err := w.cases.CasesByIDs(ctx, coverage, order)
	if err != nil {
		return dto.SearchResponse{}, err
	}

	if req.Wants(dto.SearchTypeReport) {
		resp.Results = append(resp.Results, reportResults(rows, scores)...)
	}

	if req.Wants(dto.SearchTypeKB) && w.graph != nil {
		articles, anchors, err := w.graphArticles(ctx, companyID, coverage, rows)
		if err != nil {
			return dto.SearchResponse{}, err
		}
		resp.GraphUsed = true
		resp.GraphAnchors = anchors
		resp.Results = append(resp.Results, articles...)
	}

	sort.SliceStable(resp.Results, func(i, j int) bool {
		return resp.Results[i].Score > resp.Results[j].Score
	})
	if len(resp.Results) > req.Limit {
		resp.Results = resp.Results[:req.Limit]
	}
	return resp, nil
}

func (w *Workflow) vectorReportScores(ctx context.Context, coverage []int64, vector []float32, limit int) (map[int64]float64, []int64, error) {
	scores := make(map[int64]float64)
	order := make([]int64, 0, limit)

	for _, companyID := range coverage {
		collection, err := qdrant.CollectionForCompany(w.prefix, companyID)
		if err != nil {
			return nil, nil, err
		}
		hits, err := w.vector.Search(ctx, ports.VectorSearch{
			Collection: collection,
			Query:      vector,
			Limit:      limit,
		})
		if err != nil {
			return nil, nil, err
		}
		for _, hit := range hits {
			reportID := reportIDFromHit(hit)
			if reportID <= 0 {
				continue
			}
			score := float64(hit.Score)
			if existing, seen := scores[reportID]; seen {
				if score > existing {
					scores[reportID] = score
				}
				continue
			}
			scores[reportID] = score
			order = append(order, reportID)
		}
	}
	return scores, order, nil
}

func (w *Workflow) graphArticles(ctx context.Context, companyID int64, coverage []int64, rows []CaseRecord) ([]dto.SearchResult, int, error) {
	seenSymptom := make(map[int64]struct{}, len(rows))
	seenArticle := make(map[string]struct{}, len(rows))
	out := make([]dto.SearchResult, 0, len(rows))

	for _, row := range rows {
		if row.SymptomID <= 0 {
			continue
		}
		if _, done := seenSymptom[row.SymptomID]; done {
			continue
		}
		seenSymptom[row.SymptomID] = struct{}{}

		articles, err := w.graph.ArticlesForSymptom(ctx, companyID, coverage, row.SymptomID, graphLimit)
		if err != nil {
			return nil, 0, err
		}
		for _, article := range articles {
			id := graphString(article["article_id"])
			if id == "" {
				continue
			}
			if _, dup := seenArticle[id]; dup {
				continue
			}
			seenArticle[id] = struct{}{}
			out = append(out, dto.SearchResult{
				Type:      dto.SearchTypeKB,
				ID:        "article:" + id,
				ArticleID: id,
				Title:     graphString(article["title"]),
				Source:    "graph",
				Score:     graphFloat(article["score"]),
			})
		}
	}
	return out, len(seenSymptom), nil
}

func reportResults(rows []CaseRecord, scores map[int64]float64) []dto.SearchResult {
	out := make([]dto.SearchResult, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.SearchResult{
			Type:     dto.SearchTypeReport,
			ID:       "report:" + strconv.FormatInt(row.ID, 10),
			ReportID: row.ID,
			Code:     row.Code,
			Title:    row.Title,
			Status:   row.Status,
			Source:   "vector",
			Score:    scores[row.ID],
		})
	}
	return out
}

func reportIDFromHit(hit ports.VectorHit) int64 {
	if hit.Metadata == nil {
		return 0
	}
	return toInt64(hit.Metadata["source_report_id"])
}

func toInt64(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	case string:
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0
		}
		return id
	default:
		return 0
	}
}

func graphString(value any) string {
	s, ok := value.(string)
	if !ok {
		return ""
	}
	return s
}

func graphFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int64:
		return float64(v)
	case int:
		return float64(v)
	default:
		return 0
	}
}
