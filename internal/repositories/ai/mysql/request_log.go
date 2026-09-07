package mysql

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/infra/tenant"
)

type RequestLogRepo struct {
	db tenant.Querier
}

func NewRequestLogRepo(db tenant.Querier) *RequestLogRepo {
	return &RequestLogRepo{db: db}
}

func (r *RequestLogRepo) Record(ctx context.Context, l llm.RequestLog) error {
	if l.CompanyID <= 0 {
		return errors.New("ai_request_logs: company_id must be positive")
	}
	if strings.TrimSpace(l.Vendor) == "" {
		return errors.New("ai_request_logs: vendor is required")
	}

	_, err := r.db.ExecContext(ctx, `
INSERT INTO ai_request_logs
  (company_id, vendor, model, op, status, http_status, latency_ms, input_tokens, output_tokens, error, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())`,
		l.CompanyID, l.Vendor, l.Model, l.Op, l.Status,
		nullableHTTPStatus(l.HTTPStatus), l.LatencyMs, l.InputTokens, l.OutputTokens,
		nullableErr(l.Error))
	if err != nil {
		return fmt.Errorf("ai_request_logs: insert: %w", err)
	}
	return nil
}

type UsageTotals struct {
	Requests     int
	Errors       int
	InputTokens  int
	OutputTokens int
	LatencyAvgMs int
	LatencyP50Ms int
	LatencyP95Ms int
}

type UsageToday struct {
	Requests     int
	Errors       int
	InputTokens  int
	OutputTokens int
}

type UsageDay struct {
	Date         string
	Requests     int
	Errors       int
	InputTokens  int
	OutputTokens int
	ByVendor     map[string]int
	ByStatus     map[string]int
}

type UsageModel struct {
	Vendor       string
	Model        string
	Requests     int
	Errors       int
	InputTokens  int
	OutputTokens int
	LatencyAvgMs int
}

type UsageModelDay struct {
	Date         string
	Requests     int
	Errors       int
	InputTokens  int
	OutputTokens int
}

type UsageModelDaily struct {
	Vendor string
	Model  string
	Daily  []UsageModelDay
}

type UsageRow struct {
	Timestamp    string
	Vendor       string
	Model        string
	Op           string
	Status       string
	HTTPStatus   int
	LatencyMs    int
	InputTokens  int
	OutputTokens int
	Error        string
}

type UsageSnapshot struct {
	Totals      UsageTotals
	Today       UsageToday
	Daily       []UsageDay
	ByModel     []UsageModel
	ModelsDaily []UsageModelDaily
	Recent      []UsageRow
}

func (r *RequestLogRepo) Usage(ctx context.Context, companyID int64, days int) (UsageSnapshot, error) {
	if companyID <= 0 {
		return UsageSnapshot{}, errors.New("ai_request_logs: company_id must be positive")
	}
	if days <= 0 {
		return UsageSnapshot{}, errors.New("ai_request_logs: days must be positive")
	}
	since := time.Now().AddDate(0, 0, -days+1)

	totals, err := r.usageTotals(ctx, companyID, since)
	if err != nil {
		return UsageSnapshot{}, err
	}
	today, err := r.usageToday(ctx, companyID)
	if err != nil {
		return UsageSnapshot{}, err
	}
	daily, err := r.usageDaily(ctx, companyID, since, days)
	if err != nil {
		return UsageSnapshot{}, err
	}
	byModel, err := r.usageByModel(ctx, companyID, since)
	if err != nil {
		return UsageSnapshot{}, err
	}
	modelsDaily, err := r.usageModelsDaily(ctx, companyID, since, days)
	if err != nil {
		return UsageSnapshot{}, err
	}
	recent, err := r.usageRecent(ctx, companyID)
	if err != nil {
		return UsageSnapshot{}, err
	}

	return UsageSnapshot{
		Totals:      totals,
		Today:       today,
		Daily:       daily,
		ByModel:     byModel,
		ModelsDaily: modelsDaily,
		Recent:      recent,
	}, nil
}

func (r *RequestLogRepo) usageTotals(ctx context.Context, companyID int64, since time.Time) (UsageTotals, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT
  COUNT(*),
  COALESCE(SUM(status = 'error'), 0),
  COALESCE(SUM(input_tokens), 0),
  COALESCE(SUM(output_tokens), 0),
  COALESCE(AVG(latency_ms), 0)
FROM ai_request_logs
WHERE company_id = ? AND created_at >= ?`, companyID, since)

	var totals UsageTotals
	if err := row.Scan(&totals.Requests, &totals.Errors, &totals.InputTokens, &totals.OutputTokens, &totals.LatencyAvgMs); err != nil {
		return UsageTotals{}, fmt.Errorf("ai_request_logs: totals: %w", err)
	}

	latencies, err := r.usageLatencies(ctx, companyID, since)
	if err != nil {
		return UsageTotals{}, err
	}
	totals.LatencyP50Ms = percentile(latencies, 50)
	totals.LatencyP95Ms = percentile(latencies, 95)

	return totals, nil
}

func (r *RequestLogRepo) usageLatencies(ctx context.Context, companyID int64, since time.Time) ([]int, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT latency_ms
FROM ai_request_logs
WHERE company_id = ? AND created_at >= ?
ORDER BY latency_ms
LIMIT 50000`, companyID, since)
	if err != nil {
		return nil, fmt.Errorf("ai_request_logs: latencies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]int, 0)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("ai_request_logs: latencies scan: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ai_request_logs: latencies rows: %w", err)
	}
	return out, nil
}

// percentile returns the nearest-rank percentile p (0-100) of a sorted
// ascending slice. Returns 0 when sorted is empty.
func percentile(sorted []int, p float64) int {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

func (r *RequestLogRepo) usageToday(ctx context.Context, companyID int64) (UsageToday, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT
  COUNT(*),
  COALESCE(SUM(status = 'error'), 0),
  COALESCE(SUM(input_tokens), 0),
  COALESCE(SUM(output_tokens), 0)
FROM ai_request_logs
WHERE company_id = ? AND DATE(created_at) = CURDATE()`, companyID)

	var today UsageToday
	if err := row.Scan(&today.Requests, &today.Errors, &today.InputTokens, &today.OutputTokens); err != nil {
		return UsageToday{}, fmt.Errorf("ai_request_logs: today: %w", err)
	}
	return today, nil
}

func (r *RequestLogRepo) usageDaily(ctx context.Context, companyID int64, since time.Time, days int) ([]UsageDay, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT
  DATE(created_at) AS d,
  COUNT(*),
  COALESCE(SUM(status = 'error'), 0),
  COALESCE(SUM(input_tokens), 0),
  COALESCE(SUM(output_tokens), 0)
FROM ai_request_logs
WHERE company_id = ? AND created_at >= ?
GROUP BY DATE(created_at)`, companyID, since)
	if err != nil {
		return nil, fmt.Errorf("ai_request_logs: daily: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byDate := make(map[string]UsageDay)
	for rows.Next() {
		var d UsageDay
		var date time.Time
		if err := rows.Scan(&date, &d.Requests, &d.Errors, &d.InputTokens, &d.OutputTokens); err != nil {
			return nil, fmt.Errorf("ai_request_logs: daily scan: %w", err)
		}
		d.Date = date.Format("2006-01-02")
		byDate[d.Date] = d
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ai_request_logs: daily rows: %w", err)
	}

	filled := fillDailyRange(byDate, since, days)

	byVendor, err := r.usageDailyByVendor(ctx, companyID, since)
	if err != nil {
		return nil, err
	}
	byStatus, err := r.usageDailyByStatus(ctx, companyID, since)
	if err != nil {
		return nil, err
	}
	for i := range filled {
		if v, ok := byVendor[filled[i].Date]; ok {
			filled[i].ByVendor = v
		} else {
			filled[i].ByVendor = map[string]int{}
		}
		if v, ok := byStatus[filled[i].Date]; ok {
			filled[i].ByStatus = v
		} else {
			filled[i].ByStatus = map[string]int{}
		}
	}

	return filled, nil
}

func (r *RequestLogRepo) usageDailyByVendor(ctx context.Context, companyID int64, since time.Time) (map[string]map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT DATE(created_at) AS d, vendor, COUNT(*)
FROM ai_request_logs
WHERE company_id = ? AND created_at >= ?
GROUP BY DATE(created_at), vendor`, companyID, since)
	if err != nil {
		return nil, fmt.Errorf("ai_request_logs: daily_by_vendor: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]map[string]int)
	for rows.Next() {
		var date time.Time
		var vendor string
		var count int
		if err := rows.Scan(&date, &vendor, &count); err != nil {
			return nil, fmt.Errorf("ai_request_logs: daily_by_vendor scan: %w", err)
		}
		key := date.Format("2006-01-02")
		if out[key] == nil {
			out[key] = make(map[string]int)
		}
		out[key][vendor] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ai_request_logs: daily_by_vendor rows: %w", err)
	}
	return out, nil
}

func (r *RequestLogRepo) usageDailyByStatus(ctx context.Context, companyID int64, since time.Time) (map[string]map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT
  DATE(created_at) AS d,
  CASE WHEN http_status IS NULL OR http_status = 0 THEN 'error' ELSE CAST(http_status AS CHAR) END AS status_key,
  COUNT(*)
FROM ai_request_logs
WHERE company_id = ? AND created_at >= ? AND status = 'error'
GROUP BY DATE(created_at), status_key`, companyID, since)
	if err != nil {
		return nil, fmt.Errorf("ai_request_logs: daily_by_status: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]map[string]int)
	for rows.Next() {
		var date time.Time
		var statusKey string
		var count int
		if err := rows.Scan(&date, &statusKey, &count); err != nil {
			return nil, fmt.Errorf("ai_request_logs: daily_by_status scan: %w", err)
		}
		key := date.Format("2006-01-02")
		if out[key] == nil {
			out[key] = make(map[string]int)
		}
		out[key][statusKey] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ai_request_logs: daily_by_status rows: %w", err)
	}
	return out, nil
}

func dailyDateRange(since time.Time, days int) []string {
	start := time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, since.Location())
	out := make([]string, days)
	for i := 0; i < days; i++ {
		out[i] = start.AddDate(0, 0, i).Format("2006-01-02")
	}
	return out
}

func fillDailyRange(byDate map[string]UsageDay, since time.Time, days int) []UsageDay {
	dates := dailyDateRange(since, days)
	out := make([]UsageDay, 0, days)
	for _, date := range dates {
		if d, ok := byDate[date]; ok {
			out = append(out, d)
			continue
		}
		out = append(out, UsageDay{Date: date})
	}
	return out
}

func fillModelDailyRange(byDate map[string]UsageModelDay, since time.Time, days int) []UsageModelDay {
	dates := dailyDateRange(since, days)
	out := make([]UsageModelDay, 0, days)
	for _, date := range dates {
		if d, ok := byDate[date]; ok {
			out = append(out, d)
			continue
		}
		out = append(out, UsageModelDay{Date: date})
	}
	return out
}

func (r *RequestLogRepo) usageByModel(ctx context.Context, companyID int64, since time.Time) ([]UsageModel, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT
  vendor,
  model,
  COUNT(*),
  COALESCE(SUM(status = 'error'), 0),
  COALESCE(SUM(input_tokens), 0),
  COALESCE(SUM(output_tokens), 0),
  COALESCE(AVG(latency_ms), 0)
FROM ai_request_logs
WHERE company_id = ? AND created_at >= ?
GROUP BY vendor, model
ORDER BY COUNT(*) DESC`, companyID, since)
	if err != nil {
		return nil, fmt.Errorf("ai_request_logs: by_model: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]UsageModel, 0)
	for rows.Next() {
		var m UsageModel
		if err := rows.Scan(&m.Vendor, &m.Model, &m.Requests, &m.Errors, &m.InputTokens, &m.OutputTokens, &m.LatencyAvgMs); err != nil {
			return nil, fmt.Errorf("ai_request_logs: by_model scan: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ai_request_logs: by_model rows: %w", err)
	}
	return out, nil
}

func (r *RequestLogRepo) usageModelsDaily(ctx context.Context, companyID int64, since time.Time, days int) ([]UsageModelDaily, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT
  DATE(created_at) AS d,
  vendor,
  model,
  COUNT(*),
  COALESCE(SUM(status = 'error'), 0),
  COALESCE(SUM(input_tokens), 0),
  COALESCE(SUM(output_tokens), 0)
FROM ai_request_logs
WHERE company_id = ? AND created_at >= ?
GROUP BY DATE(created_at), vendor, model`, companyID, since)
	if err != nil {
		return nil, fmt.Errorf("ai_request_logs: models_daily: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type modelKey struct{ vendor, model string }
	order := make([]modelKey, 0)
	seen := make(map[modelKey]bool)
	byKeyDate := make(map[modelKey]map[string]UsageModelDay)

	for rows.Next() {
		var date time.Time
		var k modelKey
		var d UsageModelDay
		if err := rows.Scan(&date, &k.vendor, &k.model, &d.Requests, &d.Errors, &d.InputTokens, &d.OutputTokens); err != nil {
			return nil, fmt.Errorf("ai_request_logs: models_daily scan: %w", err)
		}
		d.Date = date.Format("2006-01-02")
		if !seen[k] {
			seen[k] = true
			order = append(order, k)
			byKeyDate[k] = make(map[string]UsageModelDay)
		}
		byKeyDate[k][d.Date] = d
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ai_request_logs: models_daily rows: %w", err)
	}

	out := make([]UsageModelDaily, 0, len(order))
	for _, k := range order {
		out = append(out, UsageModelDaily{
			Vendor: k.vendor,
			Model:  k.model,
			Daily:  fillModelDailyRange(byKeyDate[k], since, days),
		})
	}
	return out, nil
}

func (r *RequestLogRepo) usageRecent(ctx context.Context, companyID int64) ([]UsageRow, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT created_at, vendor, model, op, status, COALESCE(http_status, 0), latency_ms, input_tokens, output_tokens, COALESCE(error, '')
FROM ai_request_logs
WHERE company_id = ?
ORDER BY created_at DESC, id DESC
LIMIT 50`, companyID)
	if err != nil {
		return nil, fmt.Errorf("ai_request_logs: recent: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]UsageRow, 0)
	for rows.Next() {
		var row UsageRow
		var ts time.Time
		if err := rows.Scan(&ts, &row.Vendor, &row.Model, &row.Op, &row.Status, &row.HTTPStatus, &row.LatencyMs, &row.InputTokens, &row.OutputTokens, &row.Error); err != nil {
			return nil, fmt.Errorf("ai_request_logs: recent scan: %w", err)
		}
		row.Timestamp = ts.UTC().Format(time.RFC3339)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ai_request_logs: recent rows: %w", err)
	}
	return out, nil
}

func nullableHTTPStatus(v int) any {
	if v <= 0 {
		return nil
	}
	return v
}

func nullableErr(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
