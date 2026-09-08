package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/workflows/intake"
	"github.com/my/app/internal/application/workflows/omnichannel"
	"github.com/my/app/internal/gmailconn"
	"github.com/my/app/internal/infra/tenant"
	"github.com/my/app/internal/shared/ctxkey"
)

type Repository struct {
	db tenant.Querier
}

func isDuplicateKey(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
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
WHERE channel = ? AND external_id = ? AND is_active = 1 AND (channel <> 'email' OR verified_at IS NOT NULL)
LIMIT 1`, channel, externalID).Scan(&acc.ID, &acc.CompanyID, &acc.Channel, &acc.ExternalID)
	if errors.Is(err, sql.ErrNoRows) {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: no active verified account for %s/%s: %w", channel, externalID, omnichannel.ErrAccountNotFound)
	}
	if err != nil {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: query: %w", err)
	}
	return acc, nil
}

// ByRoutingKey looks up the account bound to a mail-binding's +sjt-<key>
// routing tag (channel_accounts.routing_key). Email-only accounts must also
// be verified.
func (r *Repository) ByRoutingKey(ctx context.Context, key string) (omnichannel.ChannelAccount, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return omnichannel.ChannelAccount{}, errors.New("channel_accounts: routing_key required")
	}
	var acc omnichannel.ChannelAccount
	err := r.db.QueryRowContext(ctx, `
SELECT id, company_id, channel, external_id
FROM channel_accounts
WHERE routing_key = ? AND is_active = 1 AND (channel <> 'email' OR verified_at IS NOT NULL)
LIMIT 1`, key).Scan(&acc.ID, &acc.CompanyID, &acc.Channel, &acc.ExternalID)
	if errors.Is(err, sql.ErrNoRows) {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: no active verified account for routing_key %s: %w", key, omnichannel.ErrAccountNotFound)
	}
	if err != nil {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: query by routing_key: %w", err)
	}
	return acc, nil
}

// ByAlias looks up the account a human alias address (channel_aliases) maps
// to. The alias must be active and the owning account active and verified.
func (r *Repository) ByAlias(ctx context.Context, address string) (omnichannel.ChannelAccount, error) {
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return omnichannel.ChannelAccount{}, errors.New("channel_accounts: alias address required")
	}
	var acc omnichannel.ChannelAccount
	err := r.db.QueryRowContext(ctx, `
SELECT c.id, c.company_id, c.channel, c.external_id
FROM channel_accounts c
JOIN channel_aliases a ON a.channel_account_id = c.id
WHERE a.address = ? AND a.status = 'active' AND c.is_active = 1 AND c.verified_at IS NOT NULL
LIMIT 1`, address).Scan(&acc.ID, &acc.CompanyID, &acc.Channel, &acc.ExternalID)
	if errors.Is(err, sql.ErrNoRows) {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: no active account for alias %s: %w", address, omnichannel.ErrAccountNotFound)
	}
	if err != nil {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: query by alias: %w", err)
	}
	return acc, nil
}

// MarkVerified confirms the channel account that owns code, the 8-character
// value from the SJT-VERIFY-<CODE> mail Laravel sent to prove the tenant's
// forwarder is wired up. verification_code is unique per pending account, so
// the update targets a single unverified row; a zero-row update means the
// code is unknown or was already consumed.
func (r *Repository) MarkVerified(ctx context.Context, channel, code string) (omnichannel.ChannelAccount, error) {
	channel = strings.ToLower(strings.TrimSpace(channel))
	code = strings.ToUpper(strings.TrimSpace(code))
	if channel == "" || code == "" {
		return omnichannel.ChannelAccount{}, errors.New("channel_accounts: channel + verification_code required")
	}
	res, err := r.db.ExecContext(ctx, `
UPDATE channel_accounts
SET verified_at = NOW()
WHERE channel = ? AND verification_code = ? AND verified_at IS NULL
LIMIT 1`, channel, code)
	if err != nil {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: mark verified: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: mark verified rows affected: %w", err)
	}
	if affected == 0 {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: no unverified account for %s/%s: %w", channel, code, omnichannel.ErrAccountNotFound)
	}
	var acc omnichannel.ChannelAccount
	err = r.db.QueryRowContext(ctx, `
SELECT id, company_id, channel, external_id
FROM channel_accounts
WHERE channel = ? AND verification_code = ?
LIMIT 1`, channel, code).Scan(&acc.ID, &acc.CompanyID, &acc.Channel, &acc.ExternalID)
	if err != nil {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: read back verified account: %w", err)
	}
	return acc, nil
}

// StoreForwardConfirm records the provider's forwarding-confirmation code and
// link on the account that owns routingKey (credentials JSON keys
// forward_confirm_code / forward_confirm_link / forward_confirm_at), whether
// or not the account is verified yet: the confirmation always arrives first.
func (r *Repository) StoreForwardConfirm(ctx context.Context, routingKey, code, link string) (omnichannel.ChannelAccount, error) {
	routingKey = strings.ToLower(strings.TrimSpace(routingKey))
	code = strings.TrimSpace(code)
	link = strings.TrimSpace(link)
	if routingKey == "" || (code == "" && link == "") {
		return omnichannel.ChannelAccount{}, errors.New("channel_accounts: routing_key and a code or link required")
	}
	res, err := r.db.ExecContext(ctx, `
UPDATE channel_accounts
SET credentials = JSON_SET(COALESCE(credentials, JSON_OBJECT()),
    '$.forward_confirm_code', ?,
    '$.forward_confirm_link', ?,
    '$.forward_confirm_at', DATE_FORMAT(UTC_TIMESTAMP(), '%Y-%m-%dT%H:%i:%sZ'))
WHERE channel = 'email' AND routing_key = ?
LIMIT 1`, code, link, routingKey)
	if err != nil {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: store forward confirm: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: store forward confirm rows affected: %w", err)
	}
	if affected == 0 {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: no email account for routing_key %s: %w", routingKey, omnichannel.ErrAccountNotFound)
	}
	var acc omnichannel.ChannelAccount
	err = r.db.QueryRowContext(ctx, `
SELECT id, company_id, channel, external_id
FROM channel_accounts
WHERE channel = 'email' AND routing_key = ?
LIMIT 1`, routingKey).Scan(&acc.ID, &acc.CompanyID, &acc.Channel, &acc.ExternalID)
	if err != nil {
		return omnichannel.ChannelAccount{}, fmt.Errorf("channel_accounts: read back forward-confirm account: %w", err)
	}
	return acc, nil
}

// TouchInbound stamps last_inbound_at for the account that just received
// mail. Best-effort: callers treat a failure here as non-fatal to the inbound
// pipeline.
func (r *Repository) TouchInbound(ctx context.Context, accountID int64) error {
	if accountID <= 0 {
		return errors.New("channel_accounts: account_id required")
	}
	if _, err := r.db.ExecContext(ctx, `
UPDATE channel_accounts SET last_inbound_at = NOW() WHERE id = ?`, accountID); err != nil {
		return fmt.Errorf("channel_accounts: touch inbound: %w", err)
	}
	return nil
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
INSERT INTO conversations (company_id, channel_account_id, external_customer, subject, status, thread_scoped, last_message_at)
VALUES (?, ?, ?, ?, 'pending', ?, NOW())`, c.CompanyID, c.ChannelAccountID, nullableString(customer), nullableString(strings.TrimSpace(c.Subject)), !c.ForceNew)
	if err != nil {
		if !c.ForceNew && isDuplicateKey(err) {
			var id int64
			selErr := r.db.QueryRowContext(ctx, `
SELECT id FROM conversations
WHERE company_id = ? AND channel_account_id = ? AND external_customer <=> ? AND status = 'pending'
ORDER BY id DESC
LIMIT 1`, c.CompanyID, c.ChannelAccountID, nullableString(customer)).Scan(&id)
			if selErr == nil {
				_, _ = r.db.ExecContext(ctx, `UPDATE conversations SET last_message_at = NOW() WHERE id = ?`, id)
				return id, false, nil
			}
		}
		return 0, false, fmt.Errorf("conversations: insert: %w", err)
	}
	insertID, err := res.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	return insertID, true, nil
}

func (r *Repository) DeleteConversationIfEmpty(ctx context.Context, companyID, conversationID int64) error {
	if companyID <= 0 || conversationID <= 0 {
		return errors.New("conversations: company_id + conversation_id required")
	}
	_, err := r.db.ExecContext(ctx, `
DELETE c
FROM conversations c
LEFT JOIN messages m ON m.conversation_id = c.id
WHERE c.id = ?
  AND c.company_id = ?
  AND c.status = 'pending'
  AND c.report_id IS NULL
  AND m.id IS NULL`, conversationID, companyID)
	if err != nil {
		return fmt.Errorf("conversations: delete empty: %w", err)
	}
	return nil
}

func (r *Repository) LoadAssessmentDraft(ctx context.Context, companyID, conversationID int64) (omnichannel.AssessmentDraft, error) {
	if companyID <= 0 || conversationID <= 0 {
		return omnichannel.AssessmentDraft{}, errors.New("conversations: company_id + conversation_id required")
	}

	var (
		draft          omnichannel.AssessmentDraft
		externalID     string
		subject        string
		body           string
		rawPayload     []byte
		referencedCase string
	)
	err := r.db.QueryRowContext(ctx, `
SELECT c.id,
       c.company_id,
       COALESCE(c.external_customer, ''),
       COALESCE(c.subject, ''),
       m.id,
       COALESCE(m.external_id, ''),
       m.body,
       COALESCE(m.raw_payload, JSON_OBJECT()),
       COALESCE(c.intake_referenced_case, '')
FROM conversations c
JOIN messages m ON m.conversation_id = c.id
WHERE c.id = ?
  AND c.company_id = ?
  AND c.status = 'pending'
  AND c.report_id IS NULL
ORDER BY m.received_at DESC, m.id DESC
LIMIT 1`, conversationID, companyID).Scan(
		&draft.ConversationID,
		&draft.CompanyID,
		&draft.Customer,
		&subject,
		&draft.MessageID,
		&externalID,
		&body,
		&rawPayload,
		&referencedCase,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return omnichannel.AssessmentDraft{}, omnichannel.ErrDraftNotFound
	}
	if err != nil {
		return omnichannel.AssessmentDraft{}, fmt.Errorf("conversations: load assessment draft: %w", err)
	}

	var headers struct {
		ListUnsubscribe bool              `json:"list_unsubscribe"`
		AutoSubmitted   string            `json:"auto_submitted"`
		Precedence      string            `json:"precedence"`
		Attachments     []json.RawMessage `json:"attachments"`
	}
	_ = json.Unmarshal(rawPayload, &headers)

	draft.Request = dto.InboundMessageRequest{
		Channel:           omnichannel.ChannelEmail,
		ExternalMessageID: externalID,
		CustomerID:        draft.Customer,
		Subject:           subject,
		Body:              body,
	}
	draft.Signals = omnichannel.IntakeSignals{
		Sender:          draft.Customer,
		Subject:         subject,
		Body:            body,
		ListUnsubscribe: headers.ListUnsubscribe,
		AutoSubmitted:   strings.TrimSpace(headers.AutoSubmitted),
		Precedence:      strings.TrimSpace(headers.Precedence),
		HasAttachments:  len(headers.Attachments) > 0,
		ReferencedCase:  referencedCase,
	}

	return draft, nil
}

func (r *Repository) FindByExternalID(ctx context.Context, externalID string) (int64, int64, bool, error) {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return 0, 0, false, nil
	}
	companyID, ok := ctxkey.CompanyID(ctx)
	if !ok || companyID <= 0 {
		return 0, 0, false, errors.New("messages: company_id missing from context")
	}
	var convoID, msgID int64
	err := r.db.QueryRowContext(ctx, `
SELECT m.conversation_id, m.id
FROM messages m
JOIN conversations c ON c.id = m.conversation_id
WHERE m.external_id = ? AND c.company_id = ?
ORDER BY m.id ASC
LIMIT 1`, externalID, companyID).Scan(&convoID, &msgID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, fmt.Errorf("messages: find by external_id: %w", err)
	}
	return convoID, msgID, true, nil
}

// maxMessageBodyBytes keeps a stored body inside messages.body (MySQL
// MEDIUMTEXT, 16777215 bytes). MySQL runs with STRICT_TRANS_TABLES, so an
// oversized body is hard error 1406 rather than a truncation, and a rejected
// insert used to stall the mail poller behind it. Sized well above any real
// email body but far below the column, so no legitimate mail is cut; the
// untruncated original is kept in raw_payload regardless.
const maxMessageBodyBytes = 4 << 20

func clampMessageBody(body string) string {
	if len(body) <= maxMessageBodyBytes {
		return body
	}
	cut := maxMessageBodyBytes
	for cut > 0 && !utf8.RuneStart(body[cut]) {
		cut--
	}
	return body[:cut] + "\n[truncated]"
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
		m.ConversationID, nullableString(externalID), nullableString(strings.TrimSpace(m.SenderExternal)), clampMessageBody(m.Body), m.RawPayload)
	if err != nil {
		if externalID != "" && isDuplicateKey(err) {
			var existing int64
			if selErr := r.db.QueryRowContext(ctx, `
SELECT id FROM messages WHERE conversation_id = ? AND external_id = ? LIMIT 1`,
				m.ConversationID, externalID).Scan(&existing); selErr == nil {
				return existing, nil
			}
		}
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

	classification := sql.NullString{String: res.Classification, Valid: res.Classification != ""}
	reasoning := sql.NullString{String: res.Reasoning, Valid: res.Reasoning != ""}
	catalogRelated := sql.NullBool{Valid: res.CatalogRelated != nil}
	if res.CatalogRelated != nil {
		catalogRelated.Bool = *res.CatalogRelated
	}
	referencedCase := sql.NullString{String: res.ReferencedCase, Valid: res.ReferencedCase != ""}

	if _, err := r.db.ExecContext(ctx, `
UPDATE conversations
SET intake_status = ?, intake_missing = ?, intake_fields = ?, intake_score = ?, intake_reasons = ?, intake_classification = ?, intake_reasoning = ?, intake_catalog_related = ?, intake_referenced_case = ?, intake_confidence = ?
WHERE id = ?`, status, string(missing), string(fields), res.Score, string(reasons), classification, reasoning, catalogRelated, referencedCase, res.Confidence, conversationID); err != nil {
		return fmt.Errorf("conversations: write completeness: %w", err)
	}
	return nil
}

func (r *Repository) WriteBackfill(ctx context.Context, conversationID int64, customerID, siteID sql.NullInt64, source string) error {
	if conversationID <= 0 {
		return errors.New("conversations: conversation_id required")
	}

	backfillSource := sql.NullString{String: source, Valid: source != ""}

	if _, err := r.db.ExecContext(ctx, `
UPDATE conversations
SET intake_customer_id = ?, intake_site_id = ?, intake_backfill_source = ?
WHERE id = ?`, customerID, siteID, backfillSource, conversationID); err != nil {
		return fmt.Errorf("conversations: write backfill: %w", err)
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

// ListOAuthGmail returns every active, OAuth-connected Gmail channel account
// across all tenants. credentials is a native JSON column; the LIKE filter is
// a cheap pre-filter ahead of the JSON_EXTRACT so a full table scan of
// channel_accounts never has to parse JSON for LINE/other non-Gmail rows.
func (r *Repository) ListOAuthGmail(ctx context.Context) ([]gmailconn.OAuthAccount, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, company_id, external_id,
       COALESCE(JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.oauth_refresh_enc')), ''),
       COALESCE(JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.oauth_history_id')), '')
FROM channel_accounts
WHERE channel = 'email' AND is_active = 1 AND credentials LIKE '%"oauth_provider":"google"%'`)
	if err != nil {
		return nil, fmt.Errorf("channel_accounts: list oauth gmail: %w", err)
	}
	defer rows.Close()

	var out []gmailconn.OAuthAccount
	for rows.Next() {
		var acc gmailconn.OAuthAccount
		if err := rows.Scan(&acc.ID, &acc.CompanyID, &acc.ExternalID, &acc.RefreshEnc, &acc.HistoryID); err != nil {
			return nil, fmt.Errorf("channel_accounts: scan oauth gmail: %w", err)
		}
		out = append(out, acc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("channel_accounts: iterate oauth gmail: %w", err)
	}
	return out, nil
}

// SaveHistoryID stores the Gmail historyId processed through in the last
// poll, so the next poll can resume from it instead of re-scanning.
func (r *Repository) SaveHistoryID(ctx context.Context, id int64, historyID string) error {
	if id <= 0 {
		return errors.New("channel_accounts: id required")
	}
	historyID = strings.TrimSpace(historyID)
	if historyID == "" {
		return errors.New("channel_accounts: history_id required")
	}
	if _, err := r.db.ExecContext(ctx, `
UPDATE channel_accounts SET credentials = JSON_SET(credentials, '$.oauth_history_id', ?) WHERE id = ?`,
		historyID, id); err != nil {
		return fmt.Errorf("channel_accounts: save history id: %w", err)
	}
	return nil
}

// MarkOAuthState records whether an OAuth-connected account's refresh token
// is currently usable ("ok") or was rejected by Google and needs the tenant
// to reconnect through the Laravel consent flow ("needs_reconnect").
func (r *Repository) MarkOAuthState(ctx context.Context, id int64, state string) error {
	if id <= 0 {
		return errors.New("channel_accounts: id required")
	}
	state = strings.TrimSpace(state)
	if state != gmailconn.OAuthStateOK && state != gmailconn.OAuthStateNeedsReconnect {
		return fmt.Errorf("channel_accounts: invalid oauth state %q", state)
	}
	if _, err := r.db.ExecContext(ctx, `
UPDATE channel_accounts SET credentials = JSON_SET(credentials, '$.oauth_state', ?) WHERE id = ?`,
		state, id); err != nil {
		return fmt.Errorf("channel_accounts: mark oauth state: %w", err)
	}
	return nil
}

var (
	_ omnichannel.AccountResolver   = (*Repository)(nil)
	_ omnichannel.AccountVerifier   = (*Repository)(nil)
	_ omnichannel.ConversationStore = (*Repository)(nil)
	_ omnichannel.MessageStore      = (*Repository)(nil)
	_ omnichannel.BackfillWriter    = (*Repository)(nil)
	_ intake.Sink                   = (*Repository)(nil)
	_ gmailconn.AccountStore        = (*Repository)(nil)
)
