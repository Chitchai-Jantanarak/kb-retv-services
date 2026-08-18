package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/my/app/internal/application/workflows/classify"
)

type Repository struct {
	db *sql.DB

	mu       sync.RWMutex
	codeMaps map[string]map[string]int64
}

func New(db *sql.DB) *Repository {
	return &Repository{
		db: db,
		codeMaps: map[string]map[string]int64{
			"problem_types":   {},
			"severity_levels": {},
		},
	}
}

func (r *Repository) idByCode(ctx context.Context, table, label, code string) (int64, error) {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return 0, fmt.Errorf("classify repo: %s code is empty", label)
	}
	if table != "problem_types" && table != "severity_levels" {
		return 0, fmt.Errorf("classify repo: unsupported lookup table %q", table)
	}

	r.mu.RLock()
	if cache, ok := r.codeMaps[table]; ok {
		if id, ok := cache[code]; ok {
			r.mu.RUnlock()
			return id, nil
		}
	}
	r.mu.RUnlock()

	var id int64
	query := "SELECT id FROM " + table + " WHERE code = ? LIMIT 1"
	if err := r.db.QueryRowContext(ctx, query, code).Scan(&id); err != nil {
		return 0, fmt.Errorf("classify repo: %s %q: %w", label, code, err)
	}

	r.mu.Lock()
	if r.codeMaps[table] == nil {
		r.codeMaps[table] = make(map[string]int64)
	}
	r.codeMaps[table][code] = id
	r.mu.Unlock()
	return id, nil
}

func (r *Repository) UnclassifiedReports(ctx context.Context, companyID int64, limit int) ([]classify.ReportRecord, error) {
	if companyID <= 0 {
		return nil, errors.New("classify repo: company_id must be positive")
	}
	if limit <= 0 {
		limit = 25
	}

	rows, err := r.db.QueryContext(ctx, `
SELECT
  r.id,
  r.company_id,
  COALESCE(r.title, ''),
  CONCAT_WS(
    '\n',
    COALESCE(r.problem_detail, ''),
    COALESCE(r.fix_detail, '')
  )
FROM reports r
LEFT JOIN report_classification rc ON rc.report_id = r.id
WHERE r.company_id = ? AND rc.ai_classified_at IS NULL
ORDER BY r.id ASC
LIMIT ?`, companyID, limit)
	if err != nil {
		return nil, fmt.Errorf("classify repo: list reports: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]classify.ReportRecord, 0, limit)
	for rows.Next() {
		var rec classify.ReportRecord
		if err := rows.Scan(&rec.ID, &rec.CompanyID, &rec.Title, &rec.Body); err != nil {
			return nil, fmt.Errorf("classify repo: scan: %w", err)
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *Repository) ProblemTypeIDByCode(ctx context.Context, code string) (int64, error) {
	return r.idByCode(ctx, "problem_types", "problem_type", code)
}

func (r *Repository) SeverityIDByCode(ctx context.Context, code string) (int64, error) {
	return r.idByCode(ctx, "severity_levels", "severity", code)
}

func (r *Repository) KnownSymptoms(ctx context.Context, companyID int64, limit int) ([]string, error) {
	if companyID <= 0 {
		return nil, errors.New("classify repo: company_id must be positive")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT name
FROM symptom_nodes
WHERE company_id = ? AND status = 'active' AND canonical_id IS NULL
ORDER BY occurrence_count DESC, id ASC
LIMIT ?`, companyID, limit)
	if err != nil {
		return nil, fmt.Errorf("classify repo: known symptoms: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]string, 0, limit)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertSubject(ctx context.Context, companyID int64, name string) (int64, error) {
	name = strings.TrimSpace(name)
	if companyID <= 0 || name == "" {
		return 0, errors.New("classify repo: subject upsert: company_id+name required")
	}
	var id int64
	err := r.db.QueryRowContext(ctx, `
SELECT id FROM subject_nodes
WHERE company_id = ? AND parent_id IS NULL AND name = ?
LIMIT 1`, companyID, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("classify repo: subject select: %w", err)
	}
	res, err := r.db.ExecContext(ctx, `
INSERT INTO subject_nodes (company_id, parent_id, name, kind, status, sort_order, created_at)
VALUES (?, NULL, ?, 'subject', 'proposed', 0, NOW())`, companyID, name)
	if err != nil {
		return 0, fmt.Errorf("classify repo: subject insert: %w", err)
	}
	insertID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return insertID, nil
}

func (r *Repository) UpsertSymptom(ctx context.Context, companyID int64, name string, isNew bool) (int64, error) {
	name = strings.TrimSpace(name)
	if companyID <= 0 || name == "" {
		return 0, errors.New("classify repo: symptom upsert: company_id+name required")
	}
	var id int64
	err := r.db.QueryRowContext(ctx, `
SELECT COALESCE(canonical_id, id) FROM symptom_nodes
WHERE company_id = ? AND name = ? AND status IN ('active','proposed')
ORDER BY status = 'active' DESC, id ASC
LIMIT 1`, companyID, name).Scan(&id)
	if err == nil {
		_, _ = r.db.ExecContext(ctx, `UPDATE symptom_nodes SET occurrence_count = occurrence_count + 1, last_seen = NOW() WHERE id = ?`, id)
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("classify repo: symptom select: %w", err)
	}
	status := "active"
	if isNew {
		status = "proposed"
	}
	res, err := r.db.ExecContext(ctx, `
INSERT INTO symptom_nodes (company_id, name, discovered_by, status, occurrence_count, last_seen, created_at)
VALUES (?, ?, 'ai_extract', ?, 1, NOW(), NOW())`, companyID, name, status)
	if err != nil {
		return 0, fmt.Errorf("classify repo: symptom insert: %w", err)
	}
	insertID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return insertID, nil
}

const severityApplyThreshold = 0.6

func (r *Repository) WriteClassification(ctx context.Context, c classify.Classification) error {
	if c.ReportID <= 0 || c.CompanyID <= 0 {
		return errors.New("classify repo: write: report_id+company_id required")
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO report_classification (
  report_id, company_id,
  ai_problem_type_id, ai_problem_type_conf,
  ai_severity_id,     ai_severity_conf,
  ai_subject_node_id, ai_subject_conf,
  ai_symptom_node_id, ai_symptom_conf,
  ai_classified_at,   ai_model
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), ?)
ON DUPLICATE KEY UPDATE
  ai_problem_type_id   = VALUES(ai_problem_type_id),
  ai_problem_type_conf = VALUES(ai_problem_type_conf),
  ai_severity_id       = VALUES(ai_severity_id),
  ai_severity_conf     = VALUES(ai_severity_conf),
  ai_subject_node_id   = VALUES(ai_subject_node_id),
  ai_subject_conf      = VALUES(ai_subject_conf),
  ai_symptom_node_id   = VALUES(ai_symptom_node_id),
  ai_symptom_conf      = VALUES(ai_symptom_conf),
  ai_classified_at     = NOW(),
  ai_model             = VALUES(ai_model)`,
		c.ReportID, c.CompanyID,
		nullable(c.ProblemTypeID), nullableFloat(c.ProblemTypeConf),
		nullable(c.SeverityID), nullableFloat(c.SeverityConf),
		nullable(c.SubjectNodeID), nullableFloat(c.SubjectConf),
		nullable(c.SymptomNodeID), nullableFloat(c.SymptomConf),
		nullableString(c.Model))
	if err != nil {
		return fmt.Errorf("classify repo: write classification: %w", err)
	}

	if c.SeverityID > 0 && c.SeverityConf >= severityApplyThreshold {
		_, _ = r.db.ExecContext(ctx, `
UPDATE reports rep
JOIN severity_levels sl ON sl.id = ?
JOIN report_severities rs ON rs.code = sl.code
SET rep.severity_id = rs.id
WHERE rep.id = ? AND rep.company_id = ? AND rep.severity_id IS NULL`,
			c.SeverityID, c.ReportID, c.CompanyID)
	}
	return nil
}

func nullable(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func nullableFloat(v float64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func nullableString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

var (
	_ classify.ReportSource = (*Repository)(nil)
	_ classify.Lookup       = (*Repository)(nil)
	_ classify.Sink         = (*Repository)(nil)
)
