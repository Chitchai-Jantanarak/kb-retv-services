package main

import (
	"testing"

	infra_mysql "github.com/my/app/internal/infra/mysql"
	"github.com/my/app/internal/shared/config"
)

func TestBuildTenantQuerierReturnsRouter(t *testing.T) {
	cfg := config.Config{}
	cfg.MySQL.Enabled = true
	cfg.MySQL.DSN = "user:pass@tcp(127.0.0.1:3306)/db?parseTime=true"

	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	if db == nil {
		t.Skip("mysql disabled")
	}
	defer func() { _ = db.Close() }()

	q, err := buildTenantQuerier(cfg, db)
	if err != nil {
		t.Fatalf("buildTenantQuerier: %v", err)
	}
	if q == nil {
		t.Fatal("buildTenantQuerier returned nil router; ingest indexer would fall back to the central DB")
	}
}
