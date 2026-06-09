package kbbootstrap

import (
	"context"
	"testing"
)

func TestBootstrapperCreatesArticlesAndChunks(t *testing.T) {
	source := &fakeReportSource{
		reports: []ReportRecord{
			{
				ID:          7,
				CompanyID:   3,
				Code:        "REP-7",
				Title:       "Robot offline",
				ProblemFull: "Robot cannot start because network is unavailable.",
				FixProblem:  "Reconnect network and restart robot.",
			},
			{
				ID:          8,
				CompanyID:   3,
				Code:        "REP-8",
				Title:       "Tray jam",
				ProblemFull: "Tray is blocked by bracket.",
				FixProblem:  "Replace bracket.",
			},
		},
	}
	store := &fakeArticleStore{}
	bootstrapper := NewBootstrapper(source, store)

	result, err := bootstrapper.Run(context.Background(), Options{ChunkRunes: 40})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Reports != 2 {
		t.Fatalf("Reports = %d, want 2", result.Reports)
	}
	if result.Articles != 2 {
		t.Fatalf("Articles = %d, want 2", result.Articles)
	}
	if result.Chunks <= 2 {
		t.Fatalf("Chunks = %d, want more than 2", result.Chunks)
	}
	if len(store.articles) != 2 {
		t.Fatalf("stored articles = %d, want 2", len(store.articles))
	}
	if store.articles[0].Title != "REP-7 - Robot offline" {
		t.Fatalf("article title = %q, want REP-7 - Robot offline", store.articles[0].Title)
	}
	if store.articles[0].SourceReportID != 7 {
		t.Fatalf("SourceReportID = %d, want 7", store.articles[0].SourceReportID)
	}
}

func TestBootstrapperScopesByCompany(t *testing.T) {
	source := &fakeReportSource{
		reports: []ReportRecord{
			{ID: 1, CompanyID: 1, Code: "R-1", Title: "co1 first"},
			{ID: 2, CompanyID: 2, Code: "R-2", Title: "co2 leak"},
			{ID: 3, CompanyID: 1, Code: "R-3", Title: "co1 second"},
		},
	}
	store := &fakeArticleStore{}
	bootstrapper := NewBootstrapper(source, store)

	result, err := bootstrapper.Run(context.Background(), Options{CompanyID: 1, ChunkRunes: 40})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Reports != 2 {
		t.Fatalf("Reports = %d, want 2", result.Reports)
	}
	for _, art := range store.articles {
		if art.CompanyID != 1 {
			t.Fatalf("stored article companyID = %d, want 1 (cross-tenant leak)", art.CompanyID)
		}
		if art.SourceReportID == 2 {
			t.Fatalf("company 2 report leaked into company 1 batch")
		}
	}
}

func TestBootstrapperRejectsCrossTenantSourceLeak(t *testing.T) {
	source := &leakyReportSource{
		reports: []ReportRecord{
			{ID: 1, CompanyID: 1, Code: "R-1", Title: "co1"},
			{ID: 2, CompanyID: 2, Code: "R-2", Title: "co2 leaked despite filter"},
		},
	}
	store := &fakeArticleStore{}
	bootstrapper := NewBootstrapper(source, store)

	_, err := bootstrapper.Run(context.Background(), Options{CompanyID: 1, ChunkRunes: 40})
	if err == nil {
		t.Fatal("Run() error = nil, want cross-tenant rejection error")
	}
	if len(store.articles) != 0 {
		t.Fatalf("stored articles = %d, want 0 (must reject before write)", len(store.articles))
	}
}

type leakyReportSource struct {
	reports []ReportRecord
}

func (s *leakyReportSource) Reports(ctx context.Context, companyID int64, limit int) ([]ReportRecord, error) {
	return s.reports, nil
}

func TestBootstrapperCountsSkippedExistingArticles(t *testing.T) {
	source := &fakeReportSource{
		reports: []ReportRecord{{ID: 7, CompanyID: 3, Code: "REP-7", Title: "Robot offline"}},
	}
	store := &fakeArticleStore{skip: true}
	bootstrapper := NewBootstrapper(source, store)

	result, err := bootstrapper.Run(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1", result.Skipped)
	}
	if result.Articles != 0 {
		t.Fatalf("Articles = %d, want 0", result.Articles)
	}
}

type fakeReportSource struct {
	reports []ReportRecord
}

func (s *fakeReportSource) Reports(ctx context.Context, companyID int64, limit int) ([]ReportRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filtered := make([]ReportRecord, 0, len(s.reports))
	for _, r := range s.reports {
		if companyID > 0 && r.CompanyID != companyID {
			continue
		}
		filtered = append(filtered, r)
	}
	if limit > 0 && limit < len(filtered) {
		return filtered[:limit], nil
	}
	return filtered, nil
}

type fakeArticleStore struct {
	articles []ArticleRecord
	chunks   [][]string
	skip     bool
}

func (s *fakeArticleStore) InsertArticle(ctx context.Context, article ArticleRecord, chunks []string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s.skip {
		return false, nil
	}
	s.articles = append(s.articles, article)
	s.chunks = append(s.chunks, chunks)
	return true, nil
}
