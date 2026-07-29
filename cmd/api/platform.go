package main

import (
	"database/sql"
	"os"
	"strings"

	"go.uber.org/zap"

	"github.com/my/app/internal/infra/llm"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	"github.com/my/app/internal/infra/tenant"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

type platformDeps struct {
	central  *sql.DB
	pool     *tenant.Pool
	router   tenant.Querier
	resolver *llm.CompanyResolver
}

func buildPlatform(cfg config.Config, log *zap.Logger) platformDeps {
	var deps platformDeps

	central, err := infra_mysql.Open(cfg.MySQL)
	if err != nil {
		log.Fatal("open mysql", zap.Error(err))
	}
	if central == nil {
		return deps
	}
	deps.central = central
	log.Info("mysql configured")

	pool, err := tenant.NewPool(central, func(name string) (*sql.DB, error) {
		return infra_mysql.OpenForDB(cfg.MySQL, name)
	})
	if err != nil {
		log.Fatal("build tenant pool", zap.Error(err))
	}
	deps.pool = pool
	deps.router = pool.Router()
	log.Info("tenant DB-swarm router configured")

	resolver, err := llmboot.Resolver(cfg, deps.router)
	if err != nil {
		log.Warn("llm resolver not configured", zap.Error(err))
		return deps
	}
	deps.resolver = resolver
	log.Info("llm company resolver configured",
		zap.String("default_vendor", cfg.LLM.DefaultVendor),
		zap.String("default_model", cfg.LLM.DefaultModel))

	return deps
}

func (d platformDeps) Close() {
	if d.pool != nil {
		d.pool.Close()
	}
	if d.central != nil {
		_ = d.central.Close()
	}
}

func serviceJWTKeyMaterial(cfg config.Config, log *zap.Logger) string {
	if path := strings.TrimSpace(cfg.Laravel.JWTPublicKeyPath); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			log.Fatal("read service JWT public key", zap.String("path", path), zap.Error(err))
		}
		return string(raw)
	}
	if strings.TrimSpace(cfg.Laravel.JWKSURL) != "" {
		log.Warn("service JWT JWKS URL configured but not yet fetched; falling back to inline public key material")
	}
	return cfg.Laravel.JWTSecret
}
