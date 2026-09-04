package mediastore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

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
		ErrPrefix:   "media delivery",
		DefaultPath: "/api/webhooks/ai/media-store",
		BaseURL:     cfg.BaseURL,
		Path:        cfg.Path,
		Secret:      cfg.Secret,
		Timeout:     cfg.Timeout,
		Client:      cfg.Client,
	})
	if err != nil {
		return nil, err
	}
	return &Deliverer{hook: hook}, nil
}

type Payload struct {
	CompanyID         int64  `json:"company_id"`
	ConversationID    int64  `json:"conversation_id"`
	MessageID         int64  `json:"message_id"`
	ExternalMessageID string `json:"external_message_id"`
	MIMEType          string `json:"mime_type"`
	Filename          string `json:"filename,omitempty"`
	DataBase64        string `json:"data_base64"`
}

func (d *Deliverer) Deliver(ctx context.Context, payload Payload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("media delivery: encode payload: %w", err)
	}
	_, err = d.hook.PostSigned(ctx, body)
	return err
}
