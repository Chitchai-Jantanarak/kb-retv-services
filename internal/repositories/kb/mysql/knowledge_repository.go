package mysql

import (
	"context"
	"strconv"
	"strings"

	"github.com/my/app/internal/domain/kb"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/domain/retrieval"
	"github.com/my/app/internal/infra/tenant"
)

type KnowledgeRepository struct {
	db tenant.Querier
}

func NewKnowledgeRepository(db tenant.Querier) *KnowledgeRepository {
	return &KnowledgeRepository{db: db}
}

func (r *KnowledgeRepository) Search(ctx context.Context, query retrieval.Query) ([]kb.Article, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 3
	}

	text := strings.TrimSpace(query.Text)
	pattern := "%" + escapeLike(text) + "%"
	rows, err := r.db.QueryContext(
		ctx, `
SELECT
  a.id,
  a.company_id,
  COALESCE(a.title, ''),
  COALESCE(a.body, '')
FROM kb_articles a
WHERE
  a.company_id = ?
  AND a.status = 'published'
  AND (
    ? = ''
    OR a.title LIKE ? ESCAPE '!'
    OR a.body LIKE ? ESCAPE '!'
    OR EXISTS (
      SELECT 1
      FROM kb_chunks ch
      WHERE ch.kb_article_id = a.id AND ch.content LIKE ? ESCAPE '!'
    )
  )
ORDER BY a.created_at DESC, a.id DESC
LIMIT ?`,
		query.CompanyID,
		text,
		pattern,
		pattern,
		pattern,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	articles := make([]kb.Article, 0, limit)
	for rows.Next() {
		var article kb.Article
		var id int64
		var body string
		if err := rows.Scan(
			&id,
			&article.CompanyID,
			&article.Title,
			&body,
		); err != nil {
			return nil, err
		}
		article.ID = idToString(id)
		article.Content = body
		article.Summary = summarize(body, 240)
		articles = append(articles, article)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return articles, nil
}

func (r *KnowledgeRepository) ChunksByIDs(ctx context.Context, query kb.ChunkQuery) ([]kb.Chunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(query.IDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, 0, len(query.IDs))
	args := make([]any, 0, len(query.IDs)+1)
	for _, id := range query.IDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	args = append(args, query.CompanyID)

	rows, err := r.db.QueryContext(ctx, `
SELECT
  ch.id,
  a.id,
  a.company_id,
  COALESCE(a.title, ''),
  COALESCE(ch.content, '')
FROM kb_chunks ch
JOIN kb_articles a ON a.id = ch.kb_article_id
WHERE ch.id IN (`+strings.Join(placeholders, ",")+`)
  AND a.company_id = ?
  AND a.status = 'published'`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	chunks := make([]kb.Chunk, 0, len(query.IDs))
	for rows.Next() {
		var chunk kb.Chunk
		var chunkID, articleID int64
		if err := rows.Scan(
			&chunkID,
			&articleID,
			&chunk.CompanyID,
			&chunk.Title,
			&chunk.Content,
		); err != nil {
			return nil, err
		}
		chunk.ID = idToString(chunkID)
		chunk.ArticleID = idToString(articleID)
		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return chunks, nil
}

func (r *KnowledgeRepository) ParentChunksByChildIDs(ctx context.Context, query kb.ChunkQuery) ([]kb.ChunkContext, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(query.IDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, 0, len(query.IDs))
	args := make([]any, 0, len(query.IDs)+1)
	for _, id := range query.IDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	args = append(args, query.CompanyID)

	rows, err := r.db.QueryContext(ctx, `
SELECT
  c.id,
  COALESCE(p.id, c.id),
  a.id,
  a.company_id,
  COALESCE(a.title, ''),
  COALESCE(p.content, c.content, '')
FROM kb_chunks c
JOIN kb_articles a ON a.id = c.kb_article_id
LEFT JOIN kb_chunks p
  ON p.id = c.parent_chunk_id
  AND p.kb_article_id = c.kb_article_id
WHERE c.id IN (`+strings.Join(placeholders, ",")+`)
  AND a.company_id = ?
  AND a.status = 'published'
  AND c.chunk_kind = 'child'`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	contexts := make([]kb.ChunkContext, 0, len(query.IDs))
	for rows.Next() {
		var row kb.ChunkContext
		var childID, parentID, articleID int64
		if err := rows.Scan(
			&childID,
			&parentID,
			&articleID,
			&row.CompanyID,
			&row.Title,
			&row.Content,
		); err != nil {
			return nil, err
		}
		row.ChildID = idToString(childID)
		row.ParentID = idToString(parentID)
		row.ArticleID = idToString(articleID)
		contexts = append(contexts, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return contexts, nil
}

func idToString(id int64) string {
	return strconv.FormatInt(id, 10)
}

func summarize(text string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes])
}

func escapeLike(text string) string {
	text = strings.ReplaceAll(text, `!`, `!!`)
	text = strings.ReplaceAll(text, `%`, `!%`)
	text = strings.ReplaceAll(text, `_`, `!_`)
	return text
}

var (
	_ ports.KnowledgeRepository            = (*KnowledgeRepository)(nil)
	_ ports.KnowledgeChunkRepository       = (*KnowledgeRepository)(nil)
	_ ports.KnowledgeParentChunkRepository = (*KnowledgeRepository)(nil)
)
