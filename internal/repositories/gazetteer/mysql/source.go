package mysql

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/my/app/internal/infra/tenant"
	"github.com/my/app/internal/shared/ctxkey"
)

const (
	cacheTTL = 5 * time.Minute
	maxNames = 5000
)

var lookupQueries = map[string]string{
	"nodes.title":            "SELECT DISTINCT n.title FROM nodes n WHERE n.company_id IN (%s) AND n.title <> '' AND EXISTS (SELECT 1 FROM reports r WHERE r.product_node_id = n.id AND r.company_id = n.company_id)",
	"employees.name":         "SELECT DISTINCT name FROM employees WHERE company_id IN (%s) AND name <> ''",
	"customers.company_name": "SELECT DISTINCT company_name FROM customers WHERE company_id IN (%s) AND company_name <> ''",
}

type cacheEntry struct {
	names   []string
	expires time.Time
}

type Source struct {
	db    tenant.Querier
	mu    sync.Mutex
	cache map[string]cacheEntry
}

func NewSource(db tenant.Querier) *Source {
	return &Source{db: db, cache: make(map[string]cacheEntry)}
}

func (s *Source) Names(ctx context.Context, lookup string) ([]string, error) {
	tmpl, ok := lookupQueries[lookup]
	if !ok {
		return nil, fmt.Errorf("gazetteer: unknown lookup %q", lookup)
	}

	companyID, ok := ctxkey.CompanyID(ctx)
	if !ok || companyID <= 0 {
		return nil, fmt.Errorf("gazetteer: company scope is required")
	}
	coverage := ctxkey.CoverageOrSelf(ctx, companyID)

	key := cacheKey(lookup, coverage)
	s.mu.Lock()
	entry, hit := s.cache[key]
	s.mu.Unlock()
	if hit && time.Now().Before(entry.expires) {
		return entry.names, nil
	}

	names, err := s.query(ctx, tmpl, coverage)
	if err != nil {
		if hit {
			return entry.names, nil
		}
		return nil, err
	}

	s.mu.Lock()
	s.cache[key] = cacheEntry{names: names, expires: time.Now().Add(cacheTTL)}
	s.mu.Unlock()
	return names, nil
}

func (s *Source) query(ctx context.Context, tmpl string, coverage []int64) ([]string, error) {
	placeholders := make([]string, len(coverage))
	args := make([]any, 0, len(coverage))
	for i, id := range coverage {
		placeholders[i] = "?"
		args = append(args, id)
	}

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(tmpl+" LIMIT %d", strings.Join(placeholders, ","), maxNames), args...)
	if err != nil {
		return nil, fmt.Errorf("gazetteer: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	names := make([]string, 0, 64)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("gazetteer: scan: %w", err)
		}
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names, rows.Err()
}

func cacheKey(lookup string, coverage []int64) string {
	parts := make([]string, 0, len(coverage)+1)
	parts = append(parts, lookup)
	for _, id := range coverage {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, "|")
}
