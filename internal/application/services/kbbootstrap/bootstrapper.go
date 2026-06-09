package kbbootstrap

import (
	"context"
	"fmt"
	"strings"
)

type ReportSource interface {
	Reports(ctx context.Context, companyID int64, limit int) ([]ReportRecord, error)
}

type ArticleStore interface {
	InsertArticle(ctx context.Context, article ArticleRecord, chunks []string) (bool, error)
}

type ArticleRecord struct {
	SourceReportID int64
	CompanyID      int64
	Title          string
	Body           string
	Visibility     string
	Status         string
	Lang           string
}

type Options struct {
	CompanyID  int64
	Limit      int
	ChunkRunes int
}

type Result struct {
	Reports  int
	Articles int
	Chunks   int
	Skipped  int
}

type Bootstrapper struct {
	source ReportSource
	store  ArticleStore
}

func NewBootstrapper(source ReportSource, store ArticleStore) *Bootstrapper {
	return &Bootstrapper{source: source, store: store}
}

func (b *Bootstrapper) Run(ctx context.Context, opts Options) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	reports, err := b.source.Reports(ctx, opts.CompanyID, opts.Limit)
	if err != nil {
		return Result{}, err
	}

	if opts.CompanyID > 0 {
		for _, report := range reports {
			if report.CompanyID != opts.CompanyID {
				return Result{}, fmt.Errorf("kbbootstrap: report %d belongs to company %d, not %d", report.ID, report.CompanyID, opts.CompanyID)
			}
		}
	}

	result := Result{Reports: len(reports)}
	for _, report := range reports {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}

		body := BuildArticleBody(report)
		if strings.TrimSpace(body) == "" {
			result.Skipped++
			continue
		}

		body = LimitUTF8Bytes(body, MaxArticleBodyBytes)
		chunks := ChunkText(body, opts.ChunkRunes)
		created, err := b.store.InsertArticle(ctx, ArticleRecord{
			SourceReportID: report.ID,
			CompanyID:      report.CompanyID,
			Title:          BuildArticleTitle(report),
			Body:           body,
			Visibility:     "internal",
			Status:         "published",
			Lang:           "th",
		}, chunks)
		if err != nil {
			return Result{}, err
		}
		if !created {
			result.Skipped++
			continue
		}
		result.Articles++
		result.Chunks += len(chunks)
	}

	return result, nil
}
