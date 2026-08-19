package graphsync

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type GraphSink interface {
	UpsertReport(ctx context.Context, companyID int64, reportID int64, props map[string]any) error
	UpsertArticle(ctx context.Context, companyID int64, articleID int64, props map[string]any) error
	UpsertSymptom(ctx context.Context, companyID int64, symptomID int64, props map[string]any) error
	LinkArticleSolvesSymptom(ctx context.Context, companyID int64, articleID, symptomID int64, props map[string]any) error
	LinkReportProducedArticle(ctx context.Context, companyID int64, reportID, articleID int64) error
}

type ArticleRecord struct {
	ID             int64
	CompanyID      int64
	SourceReportID int64
	Title          string
	UpdatedAt      time.Time
	SymptomID      int64
	SymptomName    string
	SymptomConf    float64
}

type ArticleSource interface {
	ArticlesSince(ctx context.Context, companyID int64, since time.Time, limit int) ([]ArticleRecord, error)
}

type Options struct {
	CompanyID int64
	Since     time.Time
	Limit     int
}

type Result struct {
	Articles int
	Edges    int
	Skipped  int
}

type Workflow struct {
	source ArticleSource
	graph  GraphSink
}

type Config struct {
	Source ArticleSource
	Graph  GraphSink
}

func New(cfg Config) (*Workflow, error) {
	if cfg.Source == nil {
		return nil, errors.New("graphsync: source is required")
	}
	if cfg.Graph == nil {
		return nil, errors.New("graphsync: graph is required")
	}
	return &Workflow{source: cfg.Source, graph: cfg.Graph}, nil
}

func (w *Workflow) Run(ctx context.Context, opts Options) (Result, error) {
	if opts.CompanyID <= 0 {
		return Result{}, errors.New("graphsync: company_id must be positive")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	articles, err := w.source.ArticlesSince(ctx, opts.CompanyID, opts.Since, opts.Limit)
	if err != nil {
		return Result{}, fmt.Errorf("graphsync: list articles: %w", err)
	}

	res := Result{}
	for _, a := range articles {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if a.CompanyID != opts.CompanyID {
			return res, fmt.Errorf("graphsync: article %d belongs to company %d, not %d", a.ID, a.CompanyID, opts.CompanyID)
		}

		if a.SourceReportID > 0 {
			if err := w.graph.UpsertReport(ctx, opts.CompanyID, a.SourceReportID, nil); err != nil {
				return res, fmt.Errorf("graphsync: upsert report %d: %w", a.SourceReportID, err)
			}
		}

		articleProps := map[string]any{"title": a.Title, "source_report_id": a.SourceReportID}
		if err := w.graph.UpsertArticle(ctx, opts.CompanyID, a.ID, articleProps); err != nil {
			return res, fmt.Errorf("graphsync: upsert article %d: %w", a.ID, err)
		}
		res.Articles++

		if a.SourceReportID > 0 {
			if err := w.graph.LinkReportProducedArticle(ctx, opts.CompanyID, a.SourceReportID, a.ID); err != nil {
				return res, fmt.Errorf("graphsync: link report->article %d->%d: %w", a.SourceReportID, a.ID, err)
			}
			res.Edges++

			if a.SymptomID > 0 {
				symptomProps := map[string]any{"name": a.SymptomName}
				if err := w.graph.UpsertSymptom(ctx, opts.CompanyID, a.SymptomID, symptomProps); err != nil {
					return res, fmt.Errorf("graphsync: upsert symptom %d: %w", a.SymptomID, err)
				}
				if err := w.graph.LinkArticleSolvesSymptom(ctx, opts.CompanyID, a.ID, a.SymptomID, map[string]any{"score": a.SymptomConf}); err != nil {
					return res, fmt.Errorf("graphsync: link article->symptom %d->%d: %w", a.ID, a.SymptomID, err)
				}
				res.Edges++
			}
		} else {
			res.Skipped++
		}
	}
	return res, nil
}
