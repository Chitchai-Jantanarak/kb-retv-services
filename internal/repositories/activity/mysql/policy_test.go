package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/tenant"
)

type policySQL struct {
	value   string
	inserts int
	fail    bool
}

func (s *policySQL) Connect(context.Context) (driver.Conn, error) { return s, nil }
func (s *policySQL) Driver() driver.Driver                        { return s }
func (s *policySQL) Open(string) (driver.Conn, error)             { return s, nil }
func (s *policySQL) Close() error                                 { return nil }
func (s *policySQL) Begin() (driver.Tx, error)                    { return nil, errors.New("unused") }

func (s *policySQL) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unused") }

func (s *policySQL) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if s.fail {
		return nil, errors.New("policy unavailable")
	}
	if !strings.Contains(query, "system_configuration") || len(args) != 1 || args[0].Value != "audit.enabled" {
		return nil, errors.New("unexpected query")
	}
	return &policyRows{value: s.value}, nil
}

func (s *policySQL) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if !strings.Contains(query, "INSERT INTO activity_log") {
		return nil, errors.New("unexpected write")
	}
	s.inserts++
	return driver.RowsAffected(1), nil
}

type policyRows struct {
	value string
	done  bool
}

func (r *policyRows) Columns() []string { return []string{"value"} }
func (r *policyRows) Close() error      { return nil }
func (r *policyRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	dest[0] = r.value
	r.done = true
	return nil
}

type routedPolicyDB struct {
	*sql.DB
	central *sql.DB
}

func (r routedPolicyDB) Central() tenant.Querier { return r.central }

func TestAuditUsesCentralPolicyAndWritesOnlyToTenant(t *testing.T) {
	for _, tt := range []struct {
		name, value string
		fail        bool
		inserts     int
	}{
		{"enabled", "true", false, 1}, {"disabled", "false", false, 0}, {"missing policy preserves logging", "", true, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			centralDriver := &policySQL{value: tt.value, fail: tt.fail}
			tenantDriver := &policySQL{value: "false"}
			centralDB, tenantDB := sql.OpenDB(centralDriver), sql.OpenDB(tenantDriver)
			defer centralDB.Close()
			defer tenantDB.Close()
			err := New(routedPolicyDB{DB: tenantDB, central: centralDB}).RecordActivity(context.Background(), ports.ActivityEntry{CompanyID: 7, ActorType: "system", Action: "test"})
			if err != nil {
				t.Fatal(err)
			}
			if tenantDriver.inserts != tt.inserts || centralDriver.inserts != 0 {
				t.Fatalf("tenant inserts=%d central inserts=%d", tenantDriver.inserts, centralDriver.inserts)
			}
		})
	}
}
