package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	driver "github.com/go-sql-driver/mysql"

	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/infra/tenant"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/shared/providercrypto"
)

type RoutingRepository struct {
	db  tenant.Querier
	key []byte
}

func NewRoutingRepository(db tenant.Querier, encodedKey string) *RoutingRepository {
	key, _ := providercrypto.ParseKey(encodedKey)
	return &RoutingRepository{db: db, key: key}
}

func routingCompany(ctx context.Context, companyID int64) error {
	if id, ok := ctxkey.CompanyID(ctx); !ok || id <= 0 || id != companyID {
		return apperr.New(apperr.CodeForbidden, "company context does not match")
	}
	return nil
}

func (r *RoutingRepository) RouteFor(ctx context.Context, companyID int64, task string) (llm.RouteConfig, bool, error) {
	var config llm.RouteConfig
	if err := routingCompany(ctx, companyID); err != nil {
		return config, false, err
	}
	var raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT v.config, r.active_version FROM ai_task_routes r
LEFT JOIN ai_task_route_versions v ON v.route_id = r.id AND v.company_id = r.company_id AND v.version = r.active_version
WHERE r.company_id = ? AND r.task = ? AND r.active_version IS NOT NULL`, companyID, task).Scan(&raw, &config.Version)
	var mysqlErr *driver.MySQLError
	// NOTE: Unmigrated tenants retain their legacy configuration.
	if errors.Is(err, sql.ErrNoRows) || (errors.As(err, &mysqlErr) && mysqlErr.Number == 1146 && strings.Contains(mysqlErr.Message, "ai_task_routes")) {
		return config, false, nil
	}
	if err != nil {
		return config, false, err
	}
	err = json.Unmarshal(raw, &config)
	return config, true, err
}

func (r *RoutingRepository) ConnectionFor(ctx context.Context, companyID, id int64) (llm.ProviderConnection, error) {
	var c llm.ProviderConnection
	if err := routingCompany(ctx, companyID); err != nil {
		return c, err
	}
	var encrypted, models, catalogError sql.NullString
	var refreshed sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT id, provider, credential_enc, credential_version, is_active, models, models_refreshed_at, models_error
FROM ai_provider_connections WHERE company_id = ? AND id = ?`, companyID, id).
		Scan(&c.ID, &c.Provider, &encrypted, &c.CredentialVersion, &c.Active, &models, &refreshed, &catalogError)
	if errors.Is(err, sql.ErrNoRows) {
		return c, apperr.New(apperr.CodeNotFound, "AI connection not found")
	}
	if err != nil {
		return c, err
	}
	if encrypted.Valid && encrypted.String != "" {
		c.APIKey, err = providercrypto.Decrypt(encrypted.String, r.key)
		if err != nil {
			return c, apperr.New(apperr.CodeUnavailable, "AI connection credential cannot be read")
		}
	}
	c.Models = []llm.ProviderModel{}
	if models.Valid {
		if err := json.Unmarshal([]byte(models.String), &c.Models); err != nil {
			return c, err
		}
	}
	if refreshed.Valid {
		c.RefreshedAt = &refreshed.Time
	}
	c.ModelsError = catalogError.String
	return c, nil
}

func (r *RoutingRepository) SaveCatalog(ctx context.Context, companyID, id int64, version int, models []llm.ProviderModel, failure string) error {
	if err := routingCompany(ctx, companyID); err != nil {
		return err
	}
	var result sql.Result
	var err error
	if failure != "" {
		result, err = r.db.ExecContext(ctx, `UPDATE ai_provider_connections SET models_error = ?, updated_at = CURRENT_TIMESTAMP WHERE company_id = ? AND id = ? AND credential_version = ?`, failure, companyID, id, version)
	} else {
		raw, marshalErr := json.Marshal(models)
		if marshalErr != nil {
			return marshalErr
		}
		result, err = r.db.ExecContext(ctx, `UPDATE ai_provider_connections SET models = ?, models_refreshed_at = CURRENT_TIMESTAMP, models_error = NULL, updated_at = CURRENT_TIMESTAMP WHERE company_id = ? AND id = ? AND credential_version = ?`, string(raw), companyID, id, version)
	}
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		var currentVersion int
		err := r.db.QueryRowContext(ctx, `SELECT credential_version FROM ai_provider_connections WHERE company_id = ? AND id = ?`, companyID, id).Scan(&currentVersion)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if errors.Is(err, sql.ErrNoRows) || currentVersion != version {
			return apperr.New(apperr.CodeConflict, "connection changed during model discovery; refresh again")
		}
	}
	return nil
}

func (r *RoutingRepository) RecordAttempt(ctx context.Context, a llm.RouteAttempt) error {
	if err := routingCompany(ctx, a.CompanyID); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO ai_route_attempts
(company_id, request_id, task, route_version, connection_id, provider, model, operation, attempt, status, latency_ms, input_tokens, output_tokens, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		a.CompanyID, a.RequestID, a.Task, a.Version, a.ConnectionID, a.Provider, a.Model, a.Operation, a.Attempt, a.Status, a.LatencyMS, a.Usage.Input, a.Usage.Output)
	return err
}
