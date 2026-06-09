package memory

import (
	"context"
	"strings"

	"github.com/my/app/internal/domain/kb"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/domain/retrieval"
)

type KnowledgeRepository struct {
	articles []kb.Article
}

func NewKnowledgeRepository(articles []kb.Article) *KnowledgeRepository {
	copied := make([]kb.Article, len(articles))
	copy(copied, articles)
	return &KnowledgeRepository{articles: copied}
}

func (r *KnowledgeRepository) Search(ctx context.Context, query retrieval.Query) ([]kb.Article, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 3
	}

	results := make([]kb.Article, 0, limit)
	queryTerms := strings.Fields(strings.ToLower(query.Text))
	for _, article := range r.articles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if query.CompanyID > 0 && article.CompanyID != query.CompanyID {
			continue
		}
		if len(queryTerms) > 0 && !containsAnyTerm(article, queryTerms) {
			continue
		}

		results = append(results, article)
		if len(results) == limit {
			break
		}
	}

	return results, nil
}

var _ ports.KnowledgeRepository = (*KnowledgeRepository)(nil)

func containsAnyTerm(article kb.Article, terms []string) bool {
	text := strings.ToLower(article.Title + " " + article.Summary)
	for _, term := range terms {
		term = strings.Trim(term, ".,!?;:")
		if len(term) < 3 {
			continue
		}
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}
