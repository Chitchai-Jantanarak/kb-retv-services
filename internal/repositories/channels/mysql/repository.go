package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/application/workflows/omnichannel"
	"github.com/my/app/internal/infra/tenant"
)

type Repository struct {
	db tenant.Querier
}

func New(db tenant.Querier) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ByChannelAndExternalID(ctx context.Context, channel, externalID string) (omnichannel.ChannelAccount, error) {
	channel = strings.ToLower(strings.TrimSpace(channel))
	externalID = strings.TrimSpace(externalID)
	if channel == "" || externalID == "" {
		return omnichannel.ChannelAccount{}, errors.New("channel_accounts: channel + external_id required")
	}
	var acc omnichannel.ChannelAccount
	err := r.db.QueryRowContext(ctx, `
SELECT id, company_id, channel, external_id
FROM channel_accounts
WHERE channel = ? AND external_id = ? AND is_active = 1
LIMIT 1`, channel, externalID).Scan(&acc.ID, &acc.CompanyID, &acc.Channel, &acc.ExternalID)
	if errors.Is(err, sql.ErrNoRows) {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: no active account for %s/%s", channel, externalID)
	}
	if err != nil {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: query: %w", err)
	}
	return acc, nil
}

func (r *Repository) UpsertConversation(ctx context.Context, c omnichannel.Conversation) (int64, error) {
	if c.CompanyID <= 0 || c.ChannelAccountID <= 0 {
		return 0, errors.New("conversations: company_id + channel_account_id required")
	}
	customer := strings.TrimSpace(c.ExternalCustomer)
	var id int64
	err := r.db.QueryRowContext(ctx, `
SELECT id FROM conversations
WHERE company_id = ? AND channel_account_id = ? AND external_customer <=> ?
ORDER BY id DESC
LIMIT 1`, c.CompanyID, c.ChannelAccountID, nullableString(customer)).Scan(&id)
	if err == nil {
		_, _ = r.db.ExecContext(ctx, `UPDATE conversations SET last_message_at = NOW() WHERE id = ?`, id)
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("conversations: select: %w", err)
	}
	res, err := r.db.ExecContext(ctx, `
INSERT INTO conversations (company_id, channel_account_id, external_customer, status, last_message_at)
VALUES (?, ?, ?, 'open', NOW())`, c.CompanyID, c.ChannelAccountID, nullableString(customer))
	if err != nil {
		return 0, fmt.Errorf("conversations: insert: %w", err)
	}
	insertID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return insertID, nil
}

func (r *Repository) InsertMessage(ctx context.Context, m omnichannel.StoredMessage) (int64, error) {
	if m.ConversationID <= 0 {
		return 0, errors.New("messages: conversation_id required")
	}
	externalID := strings.TrimSpace(m.ExternalMessageID)

	if externalID != "" {
		var existing int64
		err := r.db.QueryRowContext(ctx, `
SELECT id FROM messages WHERE conversation_id = ? AND external_id = ? LIMIT 1`,
			m.ConversationID, externalID).Scan(&existing)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("messages: dedup lookup: %w", err)
		}
	}

	res, err := r.db.ExecContext(ctx, `
INSERT INTO messages (conversation_id, external_id, direction, sender_type, sender_external, body, raw_payload, received_at)
VALUES (?, ?, 'inbound', 'customer', ?, ?, ?, NOW())`,
		m.ConversationID, nullableString(externalID), nullableString(strings.TrimSpace(m.SenderExternal)), m.Body, m.RawPayload)
	if err != nil {
		return 0, fmt.Errorf("messages: insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

var (
	_ omnichannel.AccountResolver   = (*Repository)(nil)
	_ omnichannel.ConversationStore = (*Repository)(nil)
	_ omnichannel.MessageStore      = (*Repository)(nil)
)
