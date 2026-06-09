package mysql

import (
	"database/sql"
	"errors"
	"regexp"
	"time"

	_ "github.com/go-sql-driver/mysql" // register mysql driver with database/sql

	"github.com/my/app/internal/shared/config"
)

func Open(cfg config.MySQL) (*sql.DB, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.DSN == "" {
		return nil, errors.New("mysql dsn is required")
	}
	if err := validateDSN(cfg.DSN); err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, err
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	db.SetConnMaxLifetime(time.Hour)
	return db, nil
}

var dsnDBName = regexp.MustCompile(`(@[^/]+/)[^?]*`)

func OpenForDB(cfg config.MySQL, dbName string) (*sql.DB, error) {
	if dbName == "" {
		return nil, errors.New("mysql: db name is required")
	}
	if cfg.DSN == "" {
		return nil, errors.New("mysql dsn is required")
	}
	dsn := dsnDBName.ReplaceAllString(cfg.DSN, "${1}"+dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	db.SetConnMaxLifetime(time.Hour)
	return db, nil
}

var dsnAddressWithoutNetwork = regexp.MustCompile(`@[^(/?:]+:\d+/`)

func validateDSN(dsn string) error {
	if dsnAddressWithoutNetwork.MatchString(dsn) {
		return errors.New("mysql dsn must use tcp")
	}
	return nil
}
