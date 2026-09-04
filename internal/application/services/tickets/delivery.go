package tickets

import (
	"context"
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
		ErrPrefix:   "tickets delivery",
		DefaultPath: "/api/webhooks/ai/ticket-create",
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

func (d *Deliverer) Deliver(ctx context.Context, body []byte) error {
	_, err := d.hook.PostSigned(ctx, body)
	return err
}
