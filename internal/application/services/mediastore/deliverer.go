package mediastore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/my/app/internal/infra/laravelhook"
)

type DeliveryConfig = laravelhook.Config

type Deliverer struct {
	hook *laravelhook.Client
}

func NewDeliverer(cfg DeliveryConfig) (*Deliverer, error) {
	cfg.ErrPrefix = "media delivery"
	cfg.DefaultPath = "/api/webhooks/ai/media-store"
	hook, err := laravelhook.New(cfg)
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
