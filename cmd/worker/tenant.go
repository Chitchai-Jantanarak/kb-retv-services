package main

import (
	"database/sql"

	infra_mysql "github.com/my/app/internal/infra/mysql"
	"github.com/my/app/internal/infra/tenant"
	"github.com/my/app/internal/shared/config"
)

func buildTenantQuerier(cfg config.Config, db *sql.DB) (tenant.Querier, error) {
	pool, err := tenant.NewPool(db, func(name string) (*sql.DB, error) {
		return infra_mysql.OpenForDB(cfg.MySQL, name)
	})
	if err != nil {
		return nil, err
	}
	return pool.Router(), nil
}
