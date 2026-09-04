package reviewoutbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/laravelhook"
)

type DeliveryConfig struct {
	BaseURL string
	Path    string
	Secret  string
	Timeout time.Duration
	Client  *http.Client
}

type Deliverer struct {
	hook *laravelhook.Client
}

func NewDeliverer(cfg DeliveryConfig) (*Deliverer, error) {
	hook, err := laravelhook.New(laravelhook.Config{
		ErrPrefix:   "review outbox",
		DefaultPath: "/internal/review-queue",
		BaseURL:     cfg.BaseURL,
		Path:        cfg.Path,
		Secret:      cfg.Secret,
		Timeout:     cfg.Timeout,
		ReadLimit:   1024,
		Client:      cfg.Client,
	})
	if err != nil {
		return nil, err
	}
	return &Deliverer{hook: hook}, nil
}

func (d *Deliverer) PushReviewItem(ctx context.Context, item ports.ReviewOutboxItem) (int64, error) {
	if item.CompanyID <= 0 {
		return 0, errors.New("review outbox: company_id must be positive")
	}
	if strings.TrimSpace(string(item.Kind)) == "" {
		return 0, errors.New("review outbox: kind is required")
	}
	if len(item.Payload) == 0 {
		return 0, errors.New("review outbox: payload is required")
	}

	body, err := json.Marshal(map[string]any{
		"company_id":   item.CompanyID,
		"kind":         item.Kind,
		"payload":      item.Payload,
		"payload_hash": strings.TrimSpace(item.PayloadHash),
	})
	if err != nil {
		return 0, fmt.Errorf("review outbox: marshal: %w", err)
	}

	raw, err := d.hook.PostSigned(ctx, body)
	if err != nil {
		return 0, err
	}
	return decodeLaravelRef(raw), nil
}

func decodeLaravelRef(raw []byte) int64 {
	var direct struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(raw, &direct); err == nil && direct.ID > 0 {
		return direct.ID
	}
	var enveloped struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &enveloped); err == nil && enveloped.Data.ID > 0 {
		return enveloped.Data.ID
	}
	return 0
}

var _ ports.ReviewOutboxDeliverer = (*Deliverer)(nil)
