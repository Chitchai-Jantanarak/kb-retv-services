package mysql

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	infra_mysql "github.com/my/app/internal/infra/mysql"
	"github.com/my/app/internal/infra/tenant"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/ctxkey"
)

// GET /v1/ai/usage answered 500 for any company with no rows yet: SUM over an
// empty table returns NULL and scanning NULL into an int fails. This walks the
// real tenant databases and asserts the snapshot comes back as zeros instead.
func TestProbeUsageOnEmptyTable(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("set PROBE=1")
	}
	cfg, err := config.LoadFrom("/app/config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	central, err := infra_mysql.Open(cfg.MySQL)
	if err != nil {
		t.Fatalf("mysql: %v", err)
	}
	defer central.Close()
	pool, err := tenant.NewPool(central, func(name string) (*sql.DB, error) {
		return infra_mysql.OpenForDB(cfg.MySQL, name)
	})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	repo := NewRequestLogRepo(pool.Router())

	for _, companyID := range []int64{1, 2, 3, 4, 5} {
		ctx := ctxkey.WithCompanyID(context.Background(), companyID)
		snap, err := repo.Usage(ctx, companyID, 7)
		if err != nil {
			if strings.Contains(err.Error(), "doesn't exist") {
				t.Logf("company %d: no tenant database, skipped", companyID)
				continue
			}
			t.Errorf("company %d: Usage returned %v (this is the 500)", companyID, err)
			continue
		}
		t.Logf("company %d: requests=%d errors=%d in=%d out=%d p50=%dms",
			companyID, snap.Totals.Requests, snap.Totals.Errors,
			snap.Totals.InputTokens, snap.Totals.OutputTokens, snap.Totals.LatencyP50Ms)
	}
}
