package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
)

type configurationSQL struct {
	query func(string, []driver.NamedValue) (driver.Rows, error)
}

func (s *configurationSQL) Connect(context.Context) (driver.Conn, error) { return s, nil }
func (s *configurationSQL) Driver() driver.Driver                        { return s }
func (s *configurationSQL) Open(string) (driver.Conn, error)             { return s, nil }
func (s *configurationSQL) Close() error                                 { return nil }
func (s *configurationSQL) Begin() (driver.Tx, error) {
	return nil, errors.New("not supported")
}

func (s *configurationSQL) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not supported")
}

func (s *configurationSQL) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return s.query(query, args)
}

type configurationRows struct{ values []driver.Value }

func (r *configurationRows) Columns() []string { return []string{"value"} }
func (r *configurationRows) Close() error      { return nil }
func (r *configurationRows) Next(dest []driver.Value) error {
	if len(r.values) == 0 {
		return io.EOF
	}
	dest[0] = r.values[0]
	r.values = r.values[1:]
	return nil
}

func TestMailConfigurationDefaultsAndExplicitOptOut(t *testing.T) {
	for _, tc := range []struct {
		name, raw           string
		ai, create, invalid bool
	}{
		{name: "existing company", raw: `{"intent_keywords":["repair"]}`, ai: true, create: true},
		{name: "AI off", raw: `{"ai_enabled":false}`, create: true},
		{name: "manual creation", raw: `{"auto_create_enabled":false}`, ai: true},
		{name: "invalid policy", raw: `broken`, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := sql.OpenDB(&configurationSQL{query: func(query string, args []driver.NamedValue) (driver.Rows, error) {
				if len(args) != 1 || args[0].Value != int64(7) {
					t.Fatalf("company binding = %v", args)
				}
				switch {
				case strings.Contains(query, "FROM companies WHERE id = ?"):
					return &configurationRows{values: []driver.Value{tc.raw}}, nil
				case strings.Contains(query, "FROM ai_agents WHERE company_id = ? AND is_active = 1"):
					return &configurationRows{values: []driver.Value{"Service only products in our category tree."}}, nil
				default:
					t.Fatalf("unscoped query: %s", query)
					return nil, errors.New("unexpected query")
				}
			}})
			t.Cleanup(func() { _ = db.Close() })
			got, err := NewConfigurationRepository(db).ConfigurationFor(context.Background(), 7)
			if tc.invalid {
				if err == nil {
					t.Fatal("malformed policy accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.AIEnabled != tc.ai || got.AutoCreateEnabled != tc.create || got.Instructions == "" {
				t.Fatalf("configuration = %+v", got)
			}
		})
	}
}

func TestMailCatalogueBindsCompanyAndExcludesRootHeadings(t *testing.T) {
	db := sql.OpenDB(&configurationSQL{query: func(query string, args []driver.NamedValue) (driver.Rows, error) {
		if !strings.Contains(query, "FROM nodes WHERE company_id = ? AND parent_id IS NOT NULL") || len(args) != 1 || args[0].Value != int64(7) {
			t.Fatalf("unscoped category query: %s %v", query, args)
		}
		return &configurationRows{values: []driver.Value{" Bella Bot ", " "}}, nil
	}})
	t.Cleanup(func() { _ = db.Close() })
	names, err := NewConfigurationRepository(db).Products(context.Background(), 7)
	if err != nil || len(names) != 1 || names[0] != "Bella Bot" {
		t.Fatalf("catalogue = %v, %v", names, err)
	}
}

func TestMailConfigurationRejectsMissingCompanyBeforeSQL(t *testing.T) {
	repo := NewConfigurationRepository(nil)
	if _, err := repo.ConfigurationFor(context.Background(), 0); err == nil {
		t.Fatal("accepted missing company")
	}
	if _, err := repo.Products(context.Background(), -1); err == nil {
		t.Fatal("accepted invalid company")
	}
}
