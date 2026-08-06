package rag

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/my/app/internal/infra/tenant"
)

const (
	ftsWindowSize = 6
	ftsWindowStep = 3
	ftsMaxTerms   = 32
)

type FTSChunk struct {
	ChunkID   int64
	ArticleID int64
	Title     string
	Content   string
	Relevance float64
}

type FTSSource interface {
	SearchChunks(ctx context.Context, coverage []int64, query string, limit int) ([]FTSChunk, error)
}

func SnippetFromChunk(c FTSChunk, trimSpace bool, maxLen int) (title, snippet string) {
	title, snippet = c.Title, c.Content
	if trimSpace {
		title = strings.TrimSpace(title)
		snippet = strings.TrimSpace(snippet)
	}
	if maxLen > 0 && len(snippet) > maxLen {
		snippet = snippet[:maxLen]
	}
	return title, snippet
}

type FTSRetriever struct {
	source FTSSource
}

func NewFTSRetriever(source FTSSource) (*FTSRetriever, error) {
	if source == nil {
		return nil, errors.New("rag: fts retriever: source is required")
	}
	return &FTSRetriever{source: source}, nil
}

func (r *FTSRetriever) Retrieve(ctx context.Context, query Query, _ Meta) ([]Candidate, error) {
	q := strings.TrimSpace(query.Text)
	if q == "" {
		return nil, nil
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 5
	}
	rows, err := r.source.SearchChunks(ctx, queryCoverage(ctx, query, query.CompanyID), q, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		title, content := SnippetFromChunk(row, false, 0)
		out = append(out, Candidate{
			ID:        fmt.Sprintf("chunk:%d", row.ChunkID),
			ArticleID: fmt.Sprintf("%d", row.ArticleID),
			ChunkID:   fmt.Sprintf("%d", row.ChunkID),
			Title:     title,
			Content:   content,
			Source:    "fts",
			Score:     row.Relevance,
		})
	}
	return out, nil
}

func (r *FTSRetriever) RetrievalCost() RetrievalCost {
	return RetrievalCostCheap
}

var _ Retriever = (*FTSRetriever)(nil)
var _ CostAwareRetriever = (*FTSRetriever)(nil)

type MySQLFTSSource struct {
	db tenant.Querier
}

func NewMySQLFTSSource(db tenant.Querier) *MySQLFTSSource {
	return &MySQLFTSSource{db: db}
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}

func isSegmentedRune(r rune) bool {
	return r < unicode.MaxASCII || unicode.IsDigit(r)
}

func ftsSegments(text string) []string {
	segments := make([]string, 0, 8)
	var current []rune
	var segmented bool

	flush := func() {
		if len(current) > 0 {
			segments = append(segments, string(current))
			current = current[:0]
		}
	}

	for _, r := range text {
		if !isWordRune(r) {
			flush()
			continue
		}
		if len(current) > 0 && isSegmentedRune(r) != segmented {
			flush()
		}
		segmented = isSegmentedRune(r)
		current = append(current, r)
	}
	flush()
	return segments
}

func ftsTerms(text string) []string {
	terms := make([]string, 0, ftsMaxTerms)
	for _, segment := range ftsSegments(text) {
		runes := []rune(segment)
		if len(runes) < 2 {
			continue
		}
		if isSegmentedRune(runes[0]) || len(runes) <= ftsWindowSize {
			terms = append(terms, segment)
			continue
		}
		for i := 0; i+ftsWindowSize <= len(runes); i += ftsWindowStep {
			terms = append(terms, string(runes[i:i+ftsWindowSize]))
		}
	}
	if len(terms) > ftsMaxTerms {
		terms = terms[:ftsMaxTerms]
	}
	return terms
}

func buildFTSBooleanQuery(text string) string {
	terms := ftsTerms(text)
	if len(terms) == 0 {
		return ""
	}
	var b strings.Builder
	for i, term := range terms {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(`+"`)
		b.WriteString(term)
		b.WriteString(`"`)
	}
	return b.String()
}

func (s *MySQLFTSSource) SearchChunks(ctx context.Context, coverage []int64, query string, limit int) ([]FTSChunk, error) {
	companies := ftsCompanyIDs(coverage)
	if len(companies) == 0 {
		return nil, errors.New("kb_chunks FTS: company_id must be positive")
	}
	if limit <= 0 {
		limit = 5
	}
	match := buildFTSBooleanQuery(query)
	if match == "" {
		return []FTSChunk{}, nil
	}
	placeholders := make([]string, len(companies))
	args := make([]any, 0, len(companies)+3)
	args = append(args, match)
	for i, id := range companies {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, match, limit)
	rows, err := s.db.QueryContext(ctx, `
SELECT
  ch.id,
  ch.kb_article_id,
  COALESCE(a.title, ''),
  COALESCE(p.content, ch.content) AS context_content,
  MATCH(ch.content) AGAINST(? IN BOOLEAN MODE) AS relevance
FROM kb_chunks ch
JOIN kb_articles a ON a.id = ch.kb_article_id
LEFT JOIN kb_chunks p ON p.id = ch.parent_chunk_id
WHERE a.company_id IN (`+strings.Join(placeholders, ",")+`)
  AND ch.chunk_kind = 'child'
  AND MATCH(ch.content) AGAINST(? IN BOOLEAN MODE)
ORDER BY relevance DESC, ch.id ASC
LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("kb_chunks FTS: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]FTSChunk, 0, limit)
	for rows.Next() {
		var c FTSChunk
		if err := rows.Scan(&c.ChunkID, &c.ArticleID, &c.Title, &c.Content, &c.Relevance); err != nil {
			return nil, fmt.Errorf("kb_chunks FTS: scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

var _ FTSSource = (*MySQLFTSSource)(nil)
