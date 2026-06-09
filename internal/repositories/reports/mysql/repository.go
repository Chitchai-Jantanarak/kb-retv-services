package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/my/app/internal/infra/tenant"
)

type AccuracySnapshot struct {
	TotalClassified int64
	HumanConfirmed  int64
	ProblemTypeHits int64
	SubjectHits     int64
	SymptomHits     int64
	SeverityHits    int64
}

type AnswerRateSnapshot struct {
	TotalActions int64
	HighConf     int64
	LatencyP50Ms float64
}

type KnowledgeGapRow struct {
	ID              int64     `json:"id"`
	QueryText       string    `json:"query_text"`
	OccurrenceCount int64     `json:"occurrence_count"`
	FirstSeen       time.Time `json:"first_seen"`
	LastSeen        time.Time `json:"last_seen"`
	Status          string    `json:"status"`
}

type Repository struct {
	db tenant.Querier
}

func New(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) AIAccuracy(ctx context.Context, companyID int64, since time.Time) (AccuracySnapshot, error) {
	if companyID <= 0 {
		return AccuracySnapshot{}, errors.New("reports: company_id must be positive")
	}
	args := []any{companyID}
	timeFilter := ""
	if !since.IsZero() {
		timeFilter = " AND ai_classified_at >= ?"
		args = append(args, since)
	}
	query := fmt.Sprintf(`
SELECT
  COUNT(*),
  SUM(CASE WHEN human_confirmed_at IS NOT NULL THEN 1 ELSE 0 END),
  SUM(CASE WHEN ai_problem_type_id IS NOT NULL AND ai_problem_type_id = problem_type_id THEN 1 ELSE 0 END),
  SUM(CASE WHEN ai_subject_node_id IS NOT NULL AND ai_subject_node_id = subject_node_id THEN 1 ELSE 0 END),
  SUM(CASE WHEN ai_symptom_node_id IS NOT NULL AND ai_symptom_node_id = symptom_node_id THEN 1 ELSE 0 END),
  SUM(CASE WHEN ai_severity_id    IS NOT NULL AND ai_severity_id    = severity_id    THEN 1 ELSE 0 END)
FROM report_classification
WHERE company_id = ?%s`, timeFilter)

	var snap AccuracySnapshot
	var total, conf, pt, sub, sym, sev sql.NullInt64
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&total, &conf, &pt, &sub, &sym, &sev)
	if err != nil {
		return AccuracySnapshot{}, fmt.Errorf("reports: accuracy: %w", err)
	}
	snap.TotalClassified = total.Int64
	snap.HumanConfirmed = conf.Int64
	snap.ProblemTypeHits = pt.Int64
	snap.SubjectHits = sub.Int64
	snap.SymptomHits = sym.Int64
	snap.SeverityHits = sev.Int64
	return snap, nil
}

func (r *Repository) AnswerRate(ctx context.Context, companyID int64, since time.Time, threshold float64) (AnswerRateSnapshot, error) {
	if companyID <= 0 {
		return AnswerRateSnapshot{}, errors.New("reports: company_id must be positive")
	}
	if threshold <= 0 {
		threshold = 0.85
	}
	args := []any{companyID, threshold}
	timeFilter := ""
	if !since.IsZero() {
		timeFilter = " AND created_at >= ?"
		args = append(args, since)
	}

	var snap AnswerRateSnapshot
	var total, high sql.NullInt64
	var latency sql.NullFloat64
	err := r.db.QueryRowContext(ctx, fmt.Sprintf(`
SELECT
  COUNT(*),
  SUM(CASE WHEN confidence IS NOT NULL AND confidence >= ? THEN 1 ELSE 0 END),
  AVG(latency_ms)
FROM ai_actions
WHERE company_id = ? AND action_type = 'smart_reply'%s`, timeFilter),
		args...).Scan(&total, &high, &latency)
	if err != nil {
		return AnswerRateSnapshot{}, fmt.Errorf("reports: answer rate: %w", err)
	}
	snap.TotalActions = total.Int64
	snap.HighConf = high.Int64
	snap.LatencyP50Ms = latency.Float64
	return snap, nil
}

func (r *Repository) KnowledgeGaps(ctx context.Context, companyID int64, limit int) ([]KnowledgeGapRow, error) {
	if companyID <= 0 {
		return nil, errors.New("reports: company_id must be positive")
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, query_text, occurrence_count, first_seen, last_seen, status
FROM knowledge_gaps
WHERE company_id = ?
ORDER BY occurrence_count DESC, last_seen DESC
LIMIT ?`, companyID, limit)
	if err != nil {
		return nil, fmt.Errorf("reports: gaps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]KnowledgeGapRow, 0, limit)
	for rows.Next() {
		var row KnowledgeGapRow
		if err := rows.Scan(&row.ID, &row.QueryText, &row.OccurrenceCount, &row.FirstSeen, &row.LastSeen, &row.Status); err != nil {
			return nil, fmt.Errorf("reports: gaps scan: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
