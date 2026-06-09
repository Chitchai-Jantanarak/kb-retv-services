package rag

import (
	"context"
	"testing"

	"github.com/my/app/internal/domain/kb"
	"github.com/my/app/internal/domain/retrieval"
)

func TestKnowledgeRetrieverMapsRepositoryArticlesToCandidates(t *testing.T) {
	repo := &fakeKnowledgeRepository{
		articles: []kb.Article{
			{ID: "kb-1", Title: "Printer offline", Summary: "Check network", Content: "Check network and power"},
		},
	}
	retriever := NewKnowledgeRetriever(repo)

	candidates, err := retriever.Retrieve(context.Background(), Query{
		CompanyID: 42,
		Text:      "printer offline",
		Limit:     5,
	}, Meta{Intent: "troubleshooting"})
	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}

	if repo.query.CompanyID != 42 {
		t.Fatalf("CompanyID = %d, want 42", repo.query.CompanyID)
	}
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(candidates))
	}
	if candidates[0].ID != "kb-1" {
		t.Fatalf("ID = %q, want kb-1", candidates[0].ID)
	}
	if candidates[0].Source != "knowledge" {
		t.Fatalf("Source = %q, want knowledge", candidates[0].Source)
	}
}

type fakeKnowledgeRepository struct {
	query    retrieval.Query
	articles []kb.Article
}

func (r *fakeKnowledgeRepository) Search(ctx context.Context, query retrieval.Query) ([]kb.Article, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.query = query
	return r.articles, nil
}
