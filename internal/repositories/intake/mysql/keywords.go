package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/application/workflows/intake"
	"github.com/my/app/internal/infra/tenant"
)

type KeywordRepository struct {
	db tenant.Querier
}

func NewKeywordRepository(db tenant.Querier) *KeywordRepository {
	return &KeywordRepository{db: db}
}

func (r *KeywordRepository) IntentKeywords(ctx context.Context, companyID int64) ([]string, error) {
	if companyID <= 0 {
		return nil, errors.New("intake keywords: company_id must be positive")
	}

	var settings sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT intake_settings FROM companies WHERE id = ? LIMIT 1`, companyID).Scan(&settings)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || isUnknownColumnErr(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("intake keywords: load settings: %w", err)
	}
	if !settings.Valid {
		return nil, nil
	}

	return parseIntentKeywords(settings.String), nil
}

func parseIntentKeywords(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var parsed struct {
		IntentKeywords []string `json:"intent_keywords"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}

	var keywords []string
	for _, kw := range parsed.IntentKeywords {
		kw = strings.ToLower(strings.TrimSpace(kw))
		if kw == "" {
			continue
		}
		keywords = append(keywords, kw)
	}
	return keywords
}

func isUnknownColumnErr(err error) bool {
	return strings.Contains(err.Error(), "Unknown column")
}

func (r *KeywordRepository) PromoteThreshold(ctx context.Context, companyID int64) (int, error) {
	if companyID <= 0 {
		return 0, errors.New("intake threshold: company_id must be positive")
	}

	var settings sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT intake_settings FROM companies WHERE id = ? LIMIT 1`, companyID).Scan(&settings)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || isUnknownColumnErr(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("intake threshold: load settings: %w", err)
	}
	if !settings.Valid {
		return 0, nil
	}

	return parsePromoteThreshold(settings.String), nil
}

func parsePromoteThreshold(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}

	var parsed struct {
		PromoteThreshold int `json:"auto_promote_threshold"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return 0
	}
	if parsed.PromoteThreshold < 0 || parsed.PromoteThreshold > 95 {
		return 0
	}
	return parsed.PromoteThreshold
}

var _ intake.KeywordSource = (*KeywordRepository)(nil)
var _ intake.ThresholdSource = (*KeywordRepository)(nil)
