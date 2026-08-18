package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type SemanticRow struct {
	ID        int64
	Code      string
	Title     string
	Status    string
	SymptomID int64
}

func buildCasesByIDsQuery(coverage []int64, ids []int64) (string, []any) {
	if len(coverage) == 0 || len(ids) == 0 {
		return "", nil
	}

	args := make([]any, 0, len(coverage)+len(ids))
	coveragePlaceholders := make([]string, len(coverage))
	for i, id := range coverage {
		coveragePlaceholders[i] = "?"
		args = append(args, id)
	}
	idPlaceholders := make([]string, len(ids))
	for i, id := range ids {
		idPlaceholders[i] = "?"
		args = append(args, id)
	}

	q := fmt.Sprintf(`
SELECT
  r.id,
  COALESCE(r.code, ''),
  COALESCE(r.title, ''),
  COALESCE(s.code, ''),
  COALESCE(sn.canonical_id, rc.ai_symptom_node_id, 0)
FROM reports r
LEFT JOIN report_statuses s ON s.id = r.status_id
LEFT JOIN report_classification rc ON rc.report_id = r.id AND rc.company_id = r.company_id
LEFT JOIN symptom_nodes sn ON sn.id = rc.ai_symptom_node_id AND sn.company_id = r.company_id
WHERE r.company_id IN (%s) AND r.id IN (%s)`,
		strings.Join(coveragePlaceholders, ","), strings.Join(idPlaceholders, ","))

	return q, args
}

func (r *Repository) CasesByIDs(ctx context.Context, coverage []int64, ids []int64) ([]SemanticRow, error) {
	q, args := buildCasesByIDsQuery(coverage, ids)
	if q == "" {
		return []SemanticRow{}, nil
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("reports: cases by ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]SemanticRow, 0, len(ids))
	for rows.Next() {
		var row SemanticRow
		var code, title, status sql.NullString
		if err := rows.Scan(&row.ID, &code, &title, &status, &row.SymptomID); err != nil {
			return nil, fmt.Errorf("reports: cases by ids scan: %w", err)
		}
		row.Code = code.String
		row.Title = title.String
		row.Status = status.String
		out = append(out, row)
	}
	return out, rows.Err()
}
