package mediastore

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/application/dto"
)

type ContentFetcher interface {
	FetchContent(ctx context.Context, messageID string) (data []byte, mimeType string, err error)
}

type Promoter struct {
	fetcher   ContentFetcher
	deliverer *Deliverer
}

func NewPromoter(fetcher ContentFetcher, deliverer *Deliverer) *Promoter {
	return &Promoter{fetcher: fetcher, deliverer: deliverer}
}

var ErrNoContentFetcher = errors.New("media promote: no content fetcher configured")

func (p *Promoter) Promote(ctx context.Context, companyID, conversationID, messageID int64, ref dto.AttachmentRef) error {
	if p.fetcher == nil {
		return ErrNoContentFetcher
	}
	data, mime, err := p.fetcher.FetchContent(ctx, ref.ID)
	if err != nil {
		return fmt.Errorf("media promote: fetch content: %w", err)
	}
	if strings.TrimSpace(mime) == "" {
		mime = ref.MIMEType
	}
	return p.PromoteBytes(ctx, companyID, conversationID, messageID, ref.ID, mime, data)
}

func (p *Promoter) PromoteBytes(ctx context.Context, companyID, conversationID, messageID int64, externalID, mimeType string, data []byte) error {
	payload := Payload{
		CompanyID:         companyID,
		ConversationID:    conversationID,
		MessageID:         messageID,
		ExternalMessageID: externalID,
		MIMEType:          mimeType,
		DataBase64:        base64.StdEncoding.EncodeToString(data),
	}
	if err := p.deliverer.Deliver(ctx, payload); err != nil {
		return fmt.Errorf("media promote: deliver: %w", err)
	}
	return nil
}
