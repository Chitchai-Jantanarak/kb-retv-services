package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/infra/tenant"
	"github.com/my/app/internal/shared/providercrypto"
)

type AgentLookup struct {
	db          tenant.Querier
	providerKey []byte
}

func NewAgentLookup(db tenant.Querier, providerKeyB64 string) *AgentLookup {
	var key []byte
	if k, err := providercrypto.ParseKey(providerKeyB64); err == nil {
		key = k
	}
	return &AgentLookup{db: db, providerKey: key}
}

func (l *AgentLookup) AgentFor(ctx context.Context, companyID int64) (llm.AgentConfig, bool, error) {
	if companyID <= 0 {
		return llm.AgentConfig{}, false, errors.New("ai_agents: company_id must be positive")
	}

	var (
		id                  int64
		vendor, model       sql.NullString
		confidenceThreshold sql.NullFloat64
		providerConfig      sql.NullString
	)
	err := l.db.QueryRowContext(ctx, `
SELECT id, vendor, model, confidence_threshold, provider_config
FROM ai_agents
WHERE company_id = ? AND is_active = 1
ORDER BY (role = 'support_reply') DESC, updated_at DESC, id DESC
LIMIT 1`, companyID).Scan(&id, &vendor, &model, &confidenceThreshold, &providerConfig)
	if errors.Is(err, sql.ErrNoRows) {
		return llm.AgentConfig{}, false, nil
	}
	if err != nil {
		return llm.AgentConfig{}, false, err
	}

	cfg := llm.AgentConfig{
		ID:                  id,
		Vendor:              vendor.String,
		Model:               model.String,
		ConfidenceThreshold: confidenceThreshold.Float64,
	}
	if providerConfig.Valid && providerConfig.String != "" {
		l.applyProviderConfig(&cfg, providerConfig.String)
	}
	return cfg, true, nil
}

func (l *AgentLookup) applyProviderConfig(cfg *llm.AgentConfig, raw string) {
	var pc struct {
		APIKeyEnc string `json:"api_key_enc"`
		BaseURL   string `json:"base_url"`
	}
	if err := json.Unmarshal([]byte(raw), &pc); err != nil {
		return
	}
	cfg.BaseURL = pc.BaseURL
	if pc.APIKeyEnc != "" && len(l.providerKey) > 0 {
		if key, err := providercrypto.Decrypt(pc.APIKeyEnc, l.providerKey); err == nil {
			cfg.APIKey = key
		}
	}
}

var _ llm.AgentLookup = (*AgentLookup)(nil)
