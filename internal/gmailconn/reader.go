package gmailconn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-message/mail"

	"github.com/my/app/internal/mailpoll"
	"github.com/my/app/internal/shared/providercrypto"
)

// OAuthAccount is one OAuth-connected Gmail channel_accounts row.
type OAuthAccount struct {
	ID         int64
	CompanyID  int64
	ExternalID string // the bound mailbox address, channel_accounts.external_id
	RefreshEnc string // providercrypto ciphertext of the Google refresh token
	HistoryID  string // last Gmail historyId processed for this account
}

// AccountStore is the central-DB access the Gmail reader needs. Implemented
// by internal/repositories/channels/mysql.Repository.
type AccountStore interface {
	ListOAuthGmail(ctx context.Context) ([]OAuthAccount, error)
	SaveHistoryID(ctx context.Context, id int64, historyID string) error
	MarkOAuthState(ctx context.Context, id int64, state string) error
}

const (
	// OAuthStateOK marks an account whose refresh token is working.
	OAuthStateOK = "ok"
	// OAuthStateNeedsReconnect marks an account whose refresh token Google
	// rejected outright; only reconnecting through the Laravel consent flow
	// can clear it.
	OAuthStateNeedsReconnect = "needs_reconnect"
)

const seenCap = 500

// seenSet is a small fixed-capacity, insertion-ordered set used to dedupe
// message ids across polls without unbounded memory growth.
type seenSet struct {
	order []string
	set   map[string]struct{}
}

func newSeenSet() *seenSet {
	return &seenSet{set: make(map[string]struct{})}
}

func (s *seenSet) Has(id string) bool {
	_, ok := s.set[id]
	return ok
}

func (s *seenSet) Add(id string) {
	if _, ok := s.set[id]; ok {
		return
	}
	s.order = append(s.order, id)
	s.set[id] = struct{}{}
	if len(s.order) > seenCap {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.set, oldest)
	}
}

// Reader polls every OAuth-connected Gmail account on a fixed interval and
// forwards new inbound mail into the same inbound pipeline the IMAP poller
// (cmd/mailpoll) feeds.
type Reader struct {
	store              AccountStore
	tokens             *TokenSource
	client             *Client
	forwarder          *mailpoll.Forwarder
	providerKey        []byte
	interval           time.Duration
	systemMailbox      string
	maxAttachmentBytes int

	mu   sync.Mutex
	seen map[int64]*seenSet
}

// NewReader builds a Reader. systemMailbox (typically IMAP_USER, when set) is
// treated as a second self-sent address to skip, alongside each account's own
// bound mailbox.
func NewReader(store AccountStore, tokens *TokenSource, client *Client, forwarder *mailpoll.Forwarder, providerKey []byte, interval time.Duration, systemMailbox string) *Reader {
	return &Reader{
		store:              store,
		tokens:             tokens,
		client:             client,
		forwarder:          forwarder,
		providerKey:        providerKey,
		interval:           interval,
		systemMailbox:      strings.ToLower(strings.TrimSpace(systemMailbox)),
		maxAttachmentBytes: 8 * 1024 * 1024,
		seen:               make(map[int64]*seenSet),
	}
}

// Run polls every account on Reader's interval until ctx is cancelled.
func (r *Reader) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		r.pollOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Reader) pollOnce(ctx context.Context) {
	accounts, err := r.store.ListOAuthGmail(ctx)
	if err != nil {
		log.Printf("gmailconn: list oauth accounts: %v", err)
		return
	}
	for _, acc := range accounts {
		if ctx.Err() != nil {
			return
		}
		r.pollAccount(ctx, acc)
	}
}

// pollAccount handles a single account's poll tick. Errors are logged and
// swallowed: one account's failure (bad token, transient network error, a
// message that fails to parse) must never stop the other accounts in the
// batch from being polled.
func (r *Reader) pollAccount(ctx context.Context, acc OAuthAccount) {
	refreshToken, err := providercrypto.Decrypt(acc.RefreshEnc, r.providerKey)
	if err != nil {
		log.Printf("gmailconn: account %d: decrypt refresh token: %v", acc.ID, err)
		return
	}

	accessToken, err := r.tokens.AccessToken(ctx, acc.ID, refreshToken)
	if errors.Is(err, ErrNeedsReconnect) {
		if merr := r.store.MarkOAuthState(ctx, acc.ID, OAuthStateNeedsReconnect); merr != nil {
			log.Printf("gmailconn: account %d: mark needs_reconnect: %v", acc.ID, merr)
		}
		log.Printf("gmailconn: account %d needs reconnect (refresh token rejected)", acc.ID)
		return
	}
	if err != nil {
		log.Printf("gmailconn: account %d: refresh access token: %v", acc.ID, err)
		return
	}

	ids, newHistoryID, err := r.client.History(ctx, accessToken, acc.HistoryID)
	if err != nil {
		log.Printf("gmailconn: account %d: fetch history: %v", acc.ID, err)
		return
	}

	forwarded := 0
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		if r.alreadySeen(acc.ID, id) {
			continue
		}

		raw, err := r.client.Raw(ctx, accessToken, id)
		if err != nil {
			log.Printf("gmailconn: account %d: fetch message %s: %v", acc.ID, id, err)
			continue
		}

		payload, err := buildPayload(raw, acc.ExternalID, r.maxAttachmentBytes)
		if err != nil {
			log.Printf("gmailconn: account %d: parse message %s: %v", acc.ID, id, err)
			r.markSeen(acc.ID, id)
			continue
		}

		if isSelfSent(payload.From, acc.ExternalID, r.systemMailbox) {
			r.markSeen(acc.ID, id)
			continue
		}

		if err := r.forwarder.Forward(ctx, payload); err != nil {
			log.Printf("gmailconn: account %d: forward message %s: %v", acc.ID, id, err)
			continue
		}
		r.markSeen(acc.ID, id)
		forwarded++
	}

	if newHistoryID != "" && newHistoryID != acc.HistoryID {
		if err := r.store.SaveHistoryID(ctx, acc.ID, newHistoryID); err != nil {
			log.Printf("gmailconn: account %d: save history id: %v", acc.ID, err)
		}
	}
	if forwarded > 0 {
		log.Printf("gmailconn: account %d forwarded %d message(s)", acc.ID, forwarded)
	}
}

func (r *Reader) alreadySeen(accountID int64, id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.seen[accountID]
	return s != nil && s.Has(id)
}

func (r *Reader) markSeen(accountID int64, id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.seen[accountID]
	if s == nil {
		s = newSeenSet()
		r.seen[accountID] = s
	}
	s.Add(id)
}

// isSelfSent reports whether a message's From address is the account's own
// bound mailbox or the shared system mailbox (IMAP_USER, when configured) --
// i.e. mail this same pipeline sent, which must never be re-ingested as
// inbound customer mail.
func isSelfSent(from, ownAddress, systemMailbox string) bool {
	from = strings.ToLower(strings.TrimSpace(from))
	if from == "" {
		return false
	}
	if from == strings.ToLower(strings.TrimSpace(ownAddress)) {
		return true
	}
	if systemMailbox != "" && from == systemMailbox {
		return true
	}
	return false
}

// buildPayload turns a raw RFC 822 Gmail message into the same
// mailpoll.EmailPayload shape the IMAP poller produces, reusing
// mailpoll.ParseMIME for the body/attachments so both readers are byte-for-
// byte consistent on that half of the payload. ownAddress (the account's
// bound external_id) is forced into Recipients first, since the message may
// have reached this inbox through an alias or forward that the router still
// needs to see as the bound address.
func buildPayload(raw []byte, ownAddress string, maxAttachmentBytes int) (mailpoll.EmailPayload, error) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return mailpoll.EmailPayload{}, fmt.Errorf("create mail reader: %w", err)
	}

	p := mailpoll.EmailPayload{}

	if subject, err := mr.Header.Subject(); err == nil {
		p.Subject = subject
	}
	if msgID, err := mr.Header.MessageID(); err == nil && strings.TrimSpace(msgID) != "" {
		p.MessageID = wrapMsgID(msgID)
	}
	if inReplyTo, err := mr.Header.MsgIDList("In-Reply-To"); err == nil && len(inReplyTo) > 0 {
		p.InReplyTo = wrapMsgID(inReplyTo[0])
	}
	if refs, err := mr.Header.MsgIDList("References"); err == nil {
		for _, ref := range refs {
			p.References = append(p.References, wrapMsgID(ref))
		}
	}
	if date, err := mr.Header.Date(); err == nil && !date.IsZero() {
		p.Date = date.Format(time.RFC3339)
	}
	if froms, err := mr.Header.AddressList("From"); err == nil && len(froms) > 0 {
		p.From = strings.ToLower(strings.TrimSpace(froms[0].Address))
		p.FromName = froms[0].Name
	}

	recipients := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	addRecipient := func(addr string) {
		addr = strings.ToLower(strings.TrimSpace(addr))
		if addr == "" {
			return
		}
		if _, ok := seen[addr]; ok {
			return
		}
		seen[addr] = struct{}{}
		recipients = append(recipients, addr)
	}
	addRecipient(ownAddress)
	if tos, err := mr.Header.AddressList("To"); err == nil {
		for _, a := range tos {
			if p.To == "" {
				p.To = strings.ToLower(strings.TrimSpace(a.Address))
			}
			addRecipient(a.Address)
		}
	}
	if ccs, err := mr.Header.AddressList("Cc"); err == nil {
		for _, a := range ccs {
			addRecipient(a.Address)
		}
	}
	if p.To == "" {
		p.To = strings.ToLower(strings.TrimSpace(ownAddress))
	}
	p.Recipients = recipients

	parsed := mailpoll.ParseMIME(raw, maxAttachmentBytes)
	p.Body = parsed.Text
	p.BodyHTML = parsed.HTML
	p.Attachments = parsed.Attachments
	p.AutoSubmitted = parsed.AutoSubmitted
	p.ListUnsubscribe = parsed.ListUnsubscribe
	p.Precedence = parsed.Precedence
	p.DeliveredTo = parsed.DeliveredTo

	if strings.TrimSpace(p.MessageID) == "" {
		p.MessageID = fmt.Sprintf("<gmailpoll-%d@%s>", time.Now().UnixNano(), hostOf(ownAddress))
	}

	return p, nil
}

// wrapMsgID restores the angle brackets go-message/mail strips off parsed
// message ids, matching the format the IMAP envelope path (and Laravel's
// threading lookups) already expect.
func wrapMsgID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if !strings.HasPrefix(id, "<") {
		id = "<" + id
	}
	if !strings.HasSuffix(id, ">") {
		id += ">"
	}
	return id
}

func hostOf(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return addr[i+1:]
	}
	return "gmailpoll"
}
