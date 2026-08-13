package tenant

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/my/app/internal/shared/ctxkey"
	"github.com/my/app/internal/shared/debugtrace"
)

type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

type OpenFunc func(dbName string) (*sql.DB, error)

type Pool struct {
	manager *sql.DB
	open    OpenFunc
	mu      sync.RWMutex
	cache   map[string]*sql.DB
}

func NewPool(manager *sql.DB, open OpenFunc) (*Pool, error) {
	if manager == nil {
		return nil, errors.New("tenant: manager connection is required")
	}
	if open == nil {
		return nil, errors.New("tenant: open func is required")
	}
	return &Pool{manager: manager, open: open, cache: make(map[string]*sql.DB)}, nil
}

func (p *Pool) Router() *Router { return &Router{pool: p} }

func (p *Pool) Manager() *sql.DB { return p.manager }

func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for name, db := range p.cache {
		_ = db.Close()
		delete(p.cache, name)
	}
}

func (p *Pool) connFor(ctx context.Context) (db *sql.DB, err error) {
	if debugtrace.Enabled(ctx) {
		start := time.Now()
		defer func() {
			state := "resolved"
			errorCode := ""
			safeError := ""
			if err != nil {
				state = "failed"
				errorCode = "tenant_route_failed"
				safeError = "Could not resolve the company-scoped MySQL connection."
			}
			debugtrace.Add(ctx, debugtrace.Event{
				Stage:      "mysql.route",
				State:      state,
				Label:      "resolve company-scoped MySQL connection",
				DurationMS: debugtrace.Since(start),
				ErrorCode:  errorCode,
				Error:      safeError,
				Context: map[string]string{
					"scope_source":  "verified context",
					"database_name": "[hidden]",
				},
			})
		}()
	}
	cid, ok := ctxkey.CompanyID(ctx)
	if !ok || cid <= 0 {
		return nil, errors.New("tenant: company_id missing from context")
	}
	name, routed, err := p.tenantDBName(ctx, cid)
	if err != nil {
		return nil, err
	}
	if !routed {
		return p.manager, nil
	}
	return p.get(name)
}

func (p *Pool) tenantDBName(ctx context.Context, companyID int64) (string, bool, error) {
	var data string
	err := p.manager.QueryRowContext(ctx, `
SELECT t.data
FROM company_routes cr
JOIN tenants t ON t.id = cr.tenant_id
WHERE cr.company_id = ?
LIMIT 1`, companyID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("tenant: resolve route for company %d: %w", companyID, err)
	}
	var parsed struct {
		TenancyDBName string `json:"tenancy_db_name"`
	}
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		return "", false, fmt.Errorf("tenant: parse tenant data: %w", err)
	}
	if parsed.TenancyDBName == "" {
		return "", false, nil
	}
	return parsed.TenancyDBName, true, nil
}

func (p *Pool) get(name string) (*sql.DB, error) {
	p.mu.RLock()
	db := p.cache[name]
	p.mu.RUnlock()
	if db != nil {
		return db, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if db = p.cache[name]; db != nil {
		return db, nil
	}
	db, err := p.open(name)
	if err != nil {
		return nil, fmt.Errorf("tenant: open %q: %w", name, err)
	}
	p.cache[name] = db
	return db, nil
}

type Router struct {
	pool *Pool
}

func (r *Router) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	db, err := r.pool.connFor(ctx)
	if err != nil {
		return nil, err
	}
	start := sqlTraceStart(ctx)
	rows, err := db.QueryContext(ctx, query, args...)
	recordSQLTrace(ctx, "mysql.query", query, len(args), start, err)
	return rows, err
}

func (r *Router) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	db, err := r.pool.connFor(ctx)
	if err != nil {
		panic(err)
	}
	start := sqlTraceStart(ctx)
	row := db.QueryRowContext(ctx, query, args...)
	recordSQLTrace(ctx, "mysql.query_row", query, len(args), start, nil)
	return row
}

func (r *Router) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db, err := r.pool.connFor(ctx)
	if err != nil {
		return nil, err
	}
	start := sqlTraceStart(ctx)
	result, err := db.ExecContext(ctx, query, args...)
	recordSQLTrace(ctx, "mysql.exec", query, len(args), start, err)
	return result, err
}

func (r *Router) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	db, err := r.pool.connFor(ctx)
	if err != nil {
		return nil, err
	}
	start := sqlTraceStart(ctx)
	tx, err := db.BeginTx(ctx, opts)
	recordSQLTrace(ctx, "mysql.transaction", "BEGIN", 0, start, err)
	return tx, err
}

func sqlTraceStart(ctx context.Context) time.Time {
	if !debugtrace.Enabled(ctx) {
		return time.Time{}
	}
	return time.Now()
}

func recordSQLTrace(ctx context.Context, stage string, query string, parameterCount int, start time.Time, err error) {
	if !debugtrace.Enabled(ctx) {
		return
	}
	state := "completed"
	errorCode := ""
	safeError := ""
	if err != nil {
		state = "failed"
		errorCode = "mysql_failed"
		safeError = "The parameterized MySQL operation failed."
	}
	debugtrace.Add(ctx, debugtrace.Event{
		Stage:      stage,
		State:      state,
		Label:      "dispatch parameterized MySQL operation",
		DurationMS: debugtrace.Since(start),
		ErrorCode:  errorCode,
		Error:      safeError,
		Context: map[string]string{
			"operation":       sqlOperation(query),
			"parameter_count": strconv.Itoa(parameterCount),
			"raw_sql":         "[hidden]",
		},
	})
}

func sqlOperation(query string) string {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return "UNKNOWN"
	}
	return strings.ToUpper(fields[0])
}

var (
	_ Querier = (*Router)(nil)
	_ Querier = (*sql.DB)(nil)
)
