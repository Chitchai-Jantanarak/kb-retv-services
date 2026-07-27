package mediastore

import (
	"context"
	"encoding/base64"
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

func (p *Promoter) Promote(ctx context.Context, companyID, conversationID, messageID int64, ref dto.AttachmentRef) error {
	data, mime, err := p.fetcher.FetchContent(ctx, ref.ID)
	if err != nil {
		return fmt.Errorf("media promote: fetch content: %w", err)
	}
	if strings.TrimSpace(mime) == "" {
		mime = ref.MIMEType
	}

	payload := Payload{
		CompanyID:         companyID,
		ConversationID:    conversationID,
		MessageID:         messageID,
		ExternalMessageID: ref.ID,
		MIMEType:          mime,
		DataBase64:        base64.StdEncoding.EncodeToString(data),
	}
	if err := p.deliverer.Deliver(ctx, payload); err != nil {
		return fmt.Errorf("media promote: deliver: %w", err)
	}
	return nil
}
