package rag

import (
	"context"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/domain/retrieval"
)

type KnowledgeRetriever struct {
	repo ports.KnowledgeRepository
}

func NewKnowledgeRetriever(repo ports.KnowledgeRepository) *KnowledgeRetriever {
	return &KnowledgeRetriever{repo: repo}
}

func (r *KnowledgeRetriever) Retrieve(ctx context.Context, query Query, meta Meta) ([]Candidate, error) {
	articles, err := r.repo.Search(ctx, retrieval.Query{
		CompanyID: query.CompanyID,
		Coverage:  queryCoverage(ctx, query, query.CompanyID),
		Text:      query.Text,
		Limit:     query.Limit,
	})
	if err != nil {
		return nil, err
	}

	candidates := make([]Candidate, 0, len(articles))
	for _, article := range articles {
		content := article.Content
		if content == "" {
			content = article.Summary
		}
		candidates = append(candidates, Candidate{
			ID:        article.ID,
			ArticleID: article.ID,
			Title:     article.Title,
			Content:   content,
			Source:    "knowledge",
			Score:     1,
		})
	}
	return candidates, nil
}

func (r *KnowledgeRetriever) RetrievalCost() RetrievalCost {
	return RetrievalCostCheap
}

var _ Retriever = (*KnowledgeRetriever)(nil)
var _ CostAwareRetriever = (*KnowledgeRetriever)(nil)
