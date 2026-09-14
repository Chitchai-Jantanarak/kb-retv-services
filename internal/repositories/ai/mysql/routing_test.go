package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/shared/providercrypto"
)

type routingSQL struct {
	query func(string, []driver.NamedValue) (driver.Rows, error)
	exec  func(string, []driver.NamedValue) (driver.Result, error)
}

func (s *routingSQL) Connect(context.Context) (driver.Conn, error) { return s, nil }
func (s *routingSQL) Driver() driver.Driver                        { return s }
func (s *routingSQL) Open(string) (driver.Conn, error)             { return s, nil }
func (s *routingSQL) Close() error                                 { return nil }
func (s *routingSQL) Begin() (driver.Tx, error)                    { return nil, errors.New("not supported") }
func (s *routingSQL) Prepare(string) (driver.Stmt, error)          { return nil, errors.New("not supported") }
func (s *routingSQL) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return s.query(query, args)
}

func (s *routingSQL) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return s.exec(query, args)
}

type routingRows struct {
	columns  []string
	values   []driver.Value
	consumed bool
}

func (r *routingRows) Columns() []string { return r.columns }
func (r *routingRows) Close() error      { return nil }
func (r *routingRows) Next(dest []driver.Value) error {
	if r.consumed || r.values == nil {
		return io.EOF
	}
	r.consumed = true
	copy(dest, r.values)
	return nil
}

func TestRoutingRepositoryRejectsMismatchedCompanyBeforeSQL(t *testing.T) {
	repo := NewRoutingRepository(nil, "")
	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	if _, _, err := repo.RouteFor(ctx, 8, "chat"); err == nil {
		t.Fatal("cross-company route accepted")
	}
	if _, err := repo.ConnectionFor(ctx, 8, 1); err == nil {
		t.Fatal("cross-company connection accepted")
	}
	if err := repo.SaveCatalog(ctx, 8, 1, 1, nil, ""); err == nil {
		t.Fatal("cross-company catalog accepted")
	}
	if err := repo.RecordAttempt(ctx, llm.RouteAttempt{CompanyID: 8}); err == nil {
		t.Fatal("cross-company attempt accepted")
	}
}

func TestRoutingRepositoryOnlyFallsBackForAbsentRoutes(t *testing.T) {
	cases := []struct {
		name      string
		values    []driver.Value
		err       error
		wantError bool
	}{
		{name: "no active route"},
		{name: "unmigrated", err: &mysqldriver.MySQLError{Number: 1146, Message: "Table 'tenant.ai_task_routes' doesn't exist"}},
		{name: "missing versions", err: &mysqldriver.MySQLError{Number: 1146, Message: "Table 'tenant.ai_task_route_versions' doesn't exist"}, wantError: true},
		{name: "dangling active version", values: []driver.Value{nil, int64(2)}, wantError: true},
		{name: "invalid JSON", values: []driver.Value{"invalid", int64(2)}, wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := sql.OpenDB(&routingSQL{query: func(query string, args []driver.NamedValue) (driver.Rows, error) {
				if !strings.Contains(query, "LEFT JOIN") || !strings.Contains(query, "r.company_id = ?") || args[0].Value != int64(7) || args[1].Value != "chat" {
					t.Fatal("tenant route query invalid")
				}
				return &routingRows{columns: []string{"config", "version"}, values: tc.values}, tc.err
			}})
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Error(err)
				}
			})
			_, found, err := NewRoutingRepository(db, "").RouteFor(ctxkey.WithCompanyID(context.Background(), 7), 7, "chat")
			if (err != nil) != tc.wantError {
				t.Fatalf("found=%v err=%v", found, err)
			}
			if !tc.wantError && found {
				t.Fatal("absent route found")
			}
		})
	}
}

func TestRoutingRepositoryDecryptsOnlyOwnedConnection(t *testing.T) {
	key := testProviderKey()
	encrypted, err := providercrypto.Encrypt("connection-key", key)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(&routingSQL{query: func(query string, args []driver.NamedValue) (driver.Rows, error) {
		if !strings.Contains(query, "WHERE company_id = ? AND id = ?") || args[0].Value != int64(7) || args[1].Value != int64(12) {
			t.Fatal("connection query is not scoped")
		}
		return &routingRows{columns: []string{"id", "provider", "key", "version", "active", "models", "refreshed", "error"}, values: []driver.Value{int64(12), "openai", encrypted, int64(1), true, `[{"id":"catalog-model","capabilities":{}}]`, nil, nil}}, nil
	}})
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	connection, err := NewRoutingRepository(db, base64.StdEncoding.EncodeToString(key)).ConnectionFor(ctx, 7, 12)
	if err != nil || connection.APIKey != "connection-key" || len(connection.Models) != 1 {
		t.Fatalf("connection decode failed: %v", err)
	}
	if _, err := NewRoutingRepository(db, "").ConnectionFor(ctx, 7, 12); err == nil {
		t.Fatal("unreadable credential accepted")
	}
}

func TestCatalogSQLUsesCredentialVersionAndRejectsRotation(t *testing.T) {
	for _, version := range []int64{1, 2} {
		db := sql.OpenDB(&routingSQL{
			exec: func(query string, args []driver.NamedValue) (driver.Result, error) {
				if !strings.Contains(query, "company_id = ? AND id = ? AND credential_version = ?") || args[1].Value != int64(7) || args[2].Value != int64(12) || args[3].Value != int64(1) {
					t.Fatal("catalog update must use company and credential revision")
				}
				return driver.RowsAffected(0), nil
			},
			query: func(string, []driver.NamedValue) (driver.Rows, error) {
				return &routingRows{columns: []string{"version"}, values: []driver.Value{version}}, nil
			},
		})
		err := NewRoutingRepository(db, "").SaveCatalog(ctxkey.WithCompanyID(context.Background(), 7), 7, 12, 1, []llm.ProviderModel{}, "")
		if closeErr := db.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		if version == 1 && err != nil {
			t.Fatalf("unchanged update rejected: %v", err)
		}
		if version == 2 {
			appErr, ok := apperr.As(err)
			if !ok || appErr.Code != apperr.CodeConflict {
				t.Fatalf("rotation accepted: %v", err)
			}
		}
	}
}
