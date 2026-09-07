package tickets

import (
	"context"

	"github.com/my/app/internal/infra/laravelhook"
)

type DeliveryConfig = laravelhook.Config

type Deliverer struct {
	hook *laravelhook.Client
}

func NewDeliverer(cfg DeliveryConfig) (*Deliverer, error) {
	cfg.ErrPrefix = "tickets delivery"
	cfg.DefaultPath = "/api/webhooks/ai/ticket-create"
	hook, err := laravelhook.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Deliverer{hook: hook}, nil
}

func (d *Deliverer) Deliver(ctx context.Context, body []byte) error {
	_, err := d.hook.PostSigned(ctx, body)
	return err
}
