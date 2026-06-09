package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/my/app/internal/application/workflows/graphsync"
)

type ArticleSource struct {
	db *sql.DB
}

func NewArticleSource(db *sql.DB) *ArticleSource {
	return &ArticleSource{db: db}
}

func (s *ArticleSource) ArticlesSince(ctx context.Context, companyID int64, since time.Time, limit int) ([]graphsync.ArticleRecord, error) {
	if companyID <= 0 {
		return nil, errors.New("kb_articles: company_id must be positive")
	}
	if limit <= 0 {
		limit = 100
	}

	query := `
SELECT
  id,
  company_id,
  COALESCE(source_report_id, 0),
  COALESCE(title, ''),
  created_at
FROM kb_articles
WHERE company_id = ?`
	args := []any{companyID}
	if !since.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, since)
	}
	query += `
ORDER BY created_at ASC, id ASC
LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("kb_articles: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]graphsync.ArticleRecord, 0, 16)
	for rows.Next() {
		var rec graphsync.ArticleRecord
		if err := rows.Scan(&rec.ID, &rec.CompanyID, &rec.SourceReportID, &rec.Title, &rec.UpdatedAt); err != nil {
			return nil, fmt.Errorf("kb_articles: scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kb_articles: rows: %w", err)
	}
	return out, nil
}

var _ graphsync.ArticleSource = (*ArticleSource)(nil)
