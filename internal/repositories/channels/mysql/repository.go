package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/application/workflows/intake"
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
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: no active account for %s/%s: %w", channel, externalID, omnichannel.ErrAccountNotFound)
	}
	if err != nil {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: query: %w", err)
	}
	return acc, nil
}

func (r *Repository) UpsertConversation(ctx context.Context, c omnichannel.Conversation) (int64, bool, error) {
	if c.CompanyID <= 0 || c.ChannelAccountID <= 0 {
		return 0, false, errors.New("conversations: company_id + channel_account_id required")
	}
	customer := strings.TrimSpace(c.ExternalCustomer)

	if !c.ForceNew {
		var id int64
		err := r.db.QueryRowContext(ctx, `
SELECT id FROM conversations
WHERE company_id = ? AND channel_account_id = ? AND external_customer <=> ? AND status = 'pending'
ORDER BY id DESC
LIMIT 1`, c.CompanyID, c.ChannelAccountID, nullableString(customer)).Scan(&id)
		if err == nil {
			_, _ = r.db.ExecContext(ctx, `UPDATE conversations SET last_message_at = NOW() WHERE id = ?`, id)
			return id, false, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, false, fmt.Errorf("conversations: select: %w", err)
		}
	}

	res, err := r.db.ExecContext(ctx, `
INSERT INTO conversations (company_id, channel_account_id, external_customer, subject, status, last_message_at)
VALUES (?, ?, ?, ?, 'pending', NOW())`, c.CompanyID, c.ChannelAccountID, nullableString(customer), nullableString(strings.TrimSpace(c.Subject)))
	if err != nil {
		return 0, false, fmt.Errorf("conversations: insert: %w", err)
	}
	insertID, err := res.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	return insertID, true, nil
}

func (r *Repository) FindByExternalID(ctx context.Context, externalID string) (int64, int64, bool, error) {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return 0, 0, false, nil
	}
	var convoID, msgID int64
	err := r.db.QueryRowContext(ctx, `
SELECT conversation_id, id FROM messages WHERE external_id = ? ORDER BY id ASC LIMIT 1`, externalID).Scan(&convoID, &msgID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, fmt.Errorf("messages: find by external_id: %w", err)
	}
	return convoID, msgID, true, nil
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

func (r *Repository) WriteCompleteness(ctx context.Context, conversationID int64, res intake.Result) error {
	if conversationID <= 0 {
		return errors.New("conversations: conversation_id required")
	}
	status := strings.TrimSpace(res.Status)
	if status == "" {
		status = intake.StatusUnknown
	}

	missing, err := json.Marshal(nonNilStrings(res.Missing))
	if err != nil {
		return fmt.Errorf("conversations: encode intake_missing: %w", err)
	}
	fields, err := json.Marshal(nonNilFields(res.Fields))
	if err != nil {
		return fmt.Errorf("conversations: encode intake_fields: %w", err)
	}
	reasons, err := json.Marshal(nonNilStrings(res.Reasons))
	if err != nil {
		return fmt.Errorf("conversations: encode intake_reasons: %w", err)
	}

	if _, err := r.db.ExecContext(ctx, `
UPDATE conversations
SET intake_status = ?, intake_missing = ?, intake_fields = ?, intake_score = ?, intake_reasons = ?
WHERE id = ?`, status, string(missing), string(fields), res.Score, string(reasons), conversationID); err != nil {
		return fmt.Errorf("conversations: write completeness: %w", err)
	}
	return nil
}

func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func nonNilFields(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	return in
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
	_ intake.Sink                   = (*Repository)(nil)
)
