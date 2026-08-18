package mysql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/my/app/internal/application/workflows/graphsync"
	"github.com/my/app/internal/infra/tenant"
	"github.com/my/app/internal/shared/ctxkey"
)

type ArticleSource struct {
	db tenant.Querier
}

func NewArticleSource(db tenant.Querier) *ArticleSource {
	return &ArticleSource{db: db}
}

func (s *ArticleSource) ArticlesSince(ctx context.Context, companyID int64, since time.Time, limit int) ([]graphsync.ArticleRecord, error) {
	if companyID <= 0 {
		return nil, errors.New("kb_articles: company_id must be positive")
	}
	if limit <= 0 {
		limit = 100
	}
	ctx = ctxkey.WithCompanyID(ctx, companyID)

	query := `
SELECT
  a.id,
  a.company_id,
  COALESCE(a.source_report_id, 0),
  COALESCE(a.title, ''),
  a.created_at,
  COALESCE(canon.id, sn.id, 0),
  COALESCE(canon.name, sn.name, ''),
  COALESCE(rc.ai_symptom_conf, 0)
FROM kb_articles a
LEFT JOIN report_classification rc
  ON rc.report_id = a.source_report_id AND rc.company_id = a.company_id
LEFT JOIN symptom_nodes sn
  ON sn.id = rc.ai_symptom_node_id AND sn.company_id = a.company_id
LEFT JOIN symptom_nodes canon
  ON canon.id = sn.canonical_id AND canon.company_id = a.company_id
WHERE a.company_id = ?`
	args := []any{companyID}
	if !since.IsZero() {
		query += " AND a.created_at >= ?"
		args = append(args, since)
	}
	query += `
ORDER BY a.created_at ASC, a.id ASC
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
		if err := rows.Scan(&rec.ID, &rec.CompanyID, &rec.SourceReportID, &rec.Title, &rec.UpdatedAt, &rec.SymptomID, &rec.SymptomName, &rec.SymptomConf); err != nil {
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
