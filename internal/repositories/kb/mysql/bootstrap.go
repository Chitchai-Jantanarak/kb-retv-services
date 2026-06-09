package mysql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/my/app/internal/application/services/kbbootstrap"
)

type ReportSource struct {
	db *sql.DB
}

type ArticleStore struct {
	db *sql.DB
}

func NewReportSource(db *sql.DB) *ReportSource {
	return &ReportSource{db: db}
}

func NewArticleStore(db *sql.DB) *ArticleStore {
	return &ArticleStore{db: db}
}

func (s *ReportSource) Reports(ctx context.Context, companyID int64, limit int) ([]kbbootstrap.ReportRecord, error) {
	if limit < 0 {
		limit = 0
	}

	query := `
SELECT
  r.id,
  r.company_id,
  r.code,
  COALESCE(r.title, ''),
  COALESCE(c.company_name, ''),
  COALESCE(st.name, ''),
  COALESCE(rs.name, ''),
  COALESCE(rwt.name, ''),
  COALESCE(rsv.name, ''),
  COALESCE(JSON_UNQUOTE(JSON_EXTRACT(r.custom_fields, '$.problem_full')), ''),
  COALESCE(NULLIF(r.problem_detail, ''), JSON_UNQUOTE(JSON_EXTRACT(r.custom_fields, '$.problemdetail')), ''),
  COALESCE(NULLIF(r.fix_detail, ''), JSON_UNQUOTE(JSON_EXTRACT(r.custom_fields, '$.fixproblem')), '')
FROM reports r
LEFT JOIN customers c ON c.id = r.customer_id
LEFT JOIN sites st ON st.id = r.site_id
LEFT JOIN report_statuses rs ON rs.id = r.status_id
LEFT JOIN report_work_types rwt ON rwt.id = r.work_type_id
LEFT JOIN report_severities rsv ON rsv.id = r.severity_id`
	args := []any{}
	if companyID > 0 {
		query += "\nWHERE r.company_id = ?"
		args = append(args, companyID)
	}
	query += "\nORDER BY r.id ASC"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	reports := make([]kbbootstrap.ReportRecord, 0)
	for rows.Next() {
		var report kbbootstrap.ReportRecord
		if err := rows.Scan(
			&report.ID,
			&report.CompanyID,
			&report.Code,
			&report.Title,
			&report.CustomerName,
			&report.SiteName,
			&report.StatusName,
			&report.WorkTypeName,
			&report.SeverityName,
			&report.ProblemFull,
			&report.ProblemDetail,
			&report.FixProblem,
		); err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return reports, nil
}

func (s *ArticleStore) InsertArticle(ctx context.Context, article kbbootstrap.ArticleRecord, chunks []string) (bool, error) {
	if article.SourceReportID == 0 {
		return false, errors.New("source report id is required")
	}
	if article.CompanyID == 0 {
		return false, errors.New("company id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var existingID int64
	err = tx.QueryRowContext(ctx, `
SELECT id
FROM kb_articles
WHERE source_report_id = ? AND company_id = ?
LIMIT 1`, article.SourceReportID, article.CompanyID).Scan(&existingID)
	if err == nil {
		return false, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	res, err := tx.ExecContext(
		ctx, `
INSERT INTO kb_articles
  (source_report_id, title, body, visibility, status, lang, company_id)
VALUES
  (?, ?, ?, ?, ?, ?, ?)`,
		article.SourceReportID,
		article.Title,
		article.Body,
		article.Visibility,
		article.Status,
		article.Lang,
		article.CompanyID,
	)
	if err != nil {
		return false, err
	}

	articleID, err := res.LastInsertId()
	if err != nil {
		return false, err
	}

	if err := insertChunksWithParents(ctx, tx, articleID, chunks, parentGroupSize); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

const parentGroupSize = 3

func insertChunksWithParents(ctx context.Context, tx *sql.Tx, articleID int64, chunks []string, groupSize int) error {
	if groupSize <= 1 {
		for i, c := range chunks {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO kb_chunks (kb_article_id, chunk_index, content, token_count, chunk_kind)
VALUES (?, ?, ?, ?, 'child')`, articleID, i, c, len([]rune(c))); err != nil {
				return err
			}
		}
		return nil
	}

	parentIndex := 0
	for start := 0; start < len(chunks); start += groupSize {
		end := min(start+groupSize, len(chunks))
		parentContent := joinChunks(chunks[start:end])
		res, err := tx.ExecContext(ctx, `
INSERT INTO kb_chunks (kb_article_id, chunk_index, content, token_count, chunk_kind, parent_chunk_id)
VALUES (?, ?, ?, ?, 'parent', NULL)`,
			articleID, -1-parentIndex, parentContent, len([]rune(parentContent)))
		if err != nil {
			return err
		}
		parentID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for i := start; i < end; i++ {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO kb_chunks (kb_article_id, chunk_index, content, token_count, chunk_kind, parent_chunk_id)
VALUES (?, ?, ?, ?, 'child', ?)`,
				articleID, i, chunks[i], len([]rune(chunks[i])), parentID); err != nil {
				return err
			}
		}
		parentIndex++
	}
	return nil
}

func joinChunks(chunks []string) string {
	if len(chunks) == 1 {
		return chunks[0]
	}
	total := 0
	for _, c := range chunks {
		total += len(c) + 2
	}
	out := make([]byte, 0, total)
	for i, c := range chunks {
		if i > 0 {
			out = append(out, '\n', '\n')
		}
		out = append(out, c...)
	}
	return string(out)
}

var (
	_ kbbootstrap.ReportSource = (*ReportSource)(nil)
	_ kbbootstrap.ArticleStore = (*ArticleStore)(nil)
)
