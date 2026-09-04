package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"

	"github.com/my/app/internal/mailpoll"
)

var maxAttachmentBytes = 8 * 1024 * 1024

type config struct {
	host, user, pass, mailbox string
	inboundURL, secret        string
	interval                  time.Duration
	max                       int
	once                      bool
	statePath                 string
	maxAttempts               int
}

type uidState struct {
	UIDValidity uint32 `json:"uidvalidity"`
	LastUID     uint32 `json:"last_uid"`
	FailUID     uint32 `json:"fail_uid,omitempty"`
	FailCount   int    `json:"fail_count,omitempty"`
}

func loadState(path string) uidState {
	var st uidState
	b, err := os.ReadFile(path)
	if err != nil {
		return st
	}
	_ = json.Unmarshal(b, &st)
	return st
}

func saveState(path string, st uidState) {
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("mailpoll: state dir: %v", err)
		return
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Printf("mailpoll: state write: %v", err)
	}
}

func main() {
	cfg := loadConfig()
	fwd := mailpoll.NewForwarder(cfg.inboundURL, cfg.secret, &http.Client{Timeout: 15 * time.Second})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.once {
		if err := pollOnce(ctx, cfg, fwd); err != nil {
			log.Fatalf("mailpoll: %v", err)
		}
		return
	}

	log.Printf("mailpoll started: host=%s user=%s mailbox=%s interval=%s max=%d inbound=%s",
		cfg.host, cfg.user, cfg.mailbox, cfg.interval, cfg.max, cfg.inboundURL)
	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()
	for {
		if err := pollOnce(ctx, cfg, fwd); err != nil {
			log.Printf("mailpoll: poll error: %v", err)
		}
		select {
		case <-ctx.Done():
			log.Println("mailpoll: shutting down")
			return
		case <-ticker.C:
		}
	}
}

func pollOnce(ctx context.Context, cfg config, fwd *mailpoll.Forwarder) error {
	c, err := client.DialTLS(cfg.host, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer c.Logout()

	if err := c.Login(cfg.user, cfg.pass); err != nil {
		return fmt.Errorf("login: %w", err)
	}

	mbox, err := c.Select(cfg.mailbox, false)
	if err != nil {
		return fmt.Errorf("select %q: %w", cfg.mailbox, err)
	}

	st := loadState(cfg.statePath)
	if st.UIDValidity != mbox.UidValidity {
		st = uidState{UIDValidity: mbox.UidValidity}
	}

	bootstrap := st.LastUID == 0
	criteria := imap.NewSearchCriteria()
	if bootstrap {
		criteria.WithoutFlags = []string{imap.SeenFlag}
	} else {
		set := new(imap.SeqSet)
		set.AddRange(st.LastUID+1, 0)
		criteria.Uid = set
	}
	uids, err := c.UidSearch(criteria)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	slices.Sort(uids)
	if !bootstrap {
		filtered := uids[:0]
		for _, u := range uids {
			if u > st.LastUID {
				filtered = append(filtered, u)
			}
		}
		uids = filtered
	}
	if len(uids) == 0 {
		if bootstrap && mbox.UidNext > 0 {
			st.LastUID = mbox.UidNext - 1
			saveState(cfg.statePath, st)
		}
		return nil
	}
	if cfg.max > 0 && len(uids) > cfg.max {
		uids = uids[:cfg.max]
		if bootstrap {
			log.Printf("mailpoll: bootstrap capped at %d, remainder next tick", cfg.max)
		}
	}
	log.Printf("mailpoll: %d new message(s) to ingest (bootstrap=%v last_uid=%d)", len(uids), bootstrap, st.LastUID)

	section := &imap.BodySectionName{}
	for _, uid := range uids {
		if err := ctx.Err(); err != nil {
			return err
		}
		p, err := fetchPayload(c, uid, section, cfg.user)
		if err != nil {
			err = fmt.Errorf("fetch: %w", err)
		} else if ferr := fwd.Forward(ctx, p); ferr != nil {
			err = fmt.Errorf("forward: %w", ferr)
		}
		if err != nil {
			if skipUID(&st, cfg, uid, err) {
				continue
			}
			return nil
		}
		if st.FailUID == uid {
			st.FailUID, st.FailCount = 0, 0
		}
		markSeen(c, uid)
		if !bootstrap {
			st.LastUID = uid
			saveState(cfg.statePath, st)
		}
		log.Printf("mailpoll: ingested uid=%d from=%s subject=%q", uid, p.From, p.Subject)
	}
	if bootstrap && cfg.max == 0 && mbox.UidNext > 0 {
		st.LastUID = mbox.UidNext - 1
		saveState(cfg.statePath, st)
	}
	return nil
}

// skipUID counts consecutive failures for a single UID and reports whether the
// poller should give up on it and move on. Without this a message the inbound
// API permanently rejects blocks every newer message behind it forever, because
// a failed forward aborts the whole batch and the cursor never advances.
// A skipped message is left unread in the mailbox so it stays visible.
func skipUID(st *uidState, cfg config, uid uint32, cause error) bool {
	if st.FailUID != uid {
		st.FailUID, st.FailCount = uid, 0
	}
	st.FailCount++
	if st.FailCount < cfg.maxAttempts {
		log.Printf("mailpoll: uid %d failed (attempt %d/%d), retry next tick: %v",
			uid, st.FailCount, cfg.maxAttempts, cause)
		saveState(cfg.statePath, *st)
		return false
	}
	log.Printf("mailpoll: uid %d SKIPPED after %d failed attempts, left unread in mailbox: %v",
		uid, st.FailCount, cause)
	st.LastUID = uid
	st.FailUID, st.FailCount = 0, 0
	saveState(cfg.statePath, *st)
	return true
}

func fetchPayload(c *client.Client, uid uint32, section *imap.BodySectionName, fallbackTo string) (mailpoll.EmailPayload, error) {
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)

	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- c.UidFetch(seqset, []imap.FetchItem{imap.FetchEnvelope, section.FetchItem()}, messages)
	}()

	msg := <-messages
	if err := <-done; err != nil {
		return mailpoll.EmailPayload{}, err
	}
	if msg == nil {
		return mailpoll.EmailPayload{}, fmt.Errorf("empty fetch")
	}

	p := mailpoll.EmailPayload{To: fallbackTo, Recipients: []string{fallbackTo}}
	if msg.Envelope != nil {
		p.MessageID = msg.Envelope.MessageId
		p.InReplyTo = msg.Envelope.InReplyTo
		p.Subject = msg.Envelope.Subject
		if len(msg.Envelope.From) > 0 {
			p.From = address(msg.Envelope.From[0])
			p.FromName = msg.Envelope.From[0].PersonalName
		}
		if len(msg.Envelope.To) > 0 {
			p.To = address(msg.Envelope.To[0])
		}
		p.Recipients = recipients(msg.Envelope, fallbackTo)
		if !msg.Envelope.Date.IsZero() {
			p.Date = msg.Envelope.Date.Format(time.RFC3339)
		}
	}
	if strings.TrimSpace(p.MessageID) == "" {
		p.MessageID = fmt.Sprintf("<mailpoll-%d@%s>", uid, cfgHost(fallbackTo))
	}

	if body := msg.GetBody(section); body != nil {
		raw, _ := io.ReadAll(body)
		parsed := parseMessage(raw)
		p.Body = parsed.Text
		p.BodyHTML = parsed.HTML
		p.Attachments = parsed.Attachments
		p.AutoSubmitted = parsed.AutoSubmitted
		p.ListUnsubscribe = parsed.ListUnsubscribe
		p.Precedence = parsed.Precedence
	}
	return p, nil
}

type parsedMessage struct {
	Text            string
	HTML            string
	Attachments     []mailpoll.Attachment
	AutoSubmitted   string
	ListUnsubscribe bool
	Precedence      string
}

func parseMessage(raw []byte) parsedMessage {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return parsedMessage{Text: strings.TrimSpace(string(raw))}
	}

	result := parsedMessage{
		AutoSubmitted:   strings.TrimSpace(mr.Header.Get("Auto-Submitted")),
		ListUnsubscribe: strings.TrimSpace(mr.Header.Get("List-Unsubscribe")) != "",
		Precedence:      strings.TrimSpace(mr.Header.Get("Precedence")),
	}

	attachmentBytes := 0
	attachmentsCapped := false
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		switch h := part.Header.(type) {
		case *mail.InlineHeader:
			b, _ := io.ReadAll(part.Body)
			ct, _, _ := h.ContentType()
			if strings.Contains(ct, "html") {
				result.HTML = string(b)
			} else {
				result.Text = string(b)
			}
		case *mail.AttachmentHeader:
			if attachmentsCapped {
				continue
			}
			b, _ := io.ReadAll(part.Body)
			if attachmentBytes+len(b) > maxAttachmentBytes {
				attachmentsCapped = true
				continue
			}
			attachmentBytes += len(b)
			filename, _ := h.Filename()
			mimeType, _, _ := h.ContentType()
			result.Attachments = append(result.Attachments, mailpoll.Attachment{
				Filename:   filename,
				MIMEType:   mimeType,
				SizeBytes:  len(b),
				ContentB64: base64.StdEncoding.EncodeToString(b),
			})
		}
	}
	result.Text = strings.TrimSpace(result.Text)
	result.HTML = strings.TrimSpace(result.HTML)
	return result
}

func markSeen(c *client.Client, uid uint32) {
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	_ = c.UidStore(seqset, item, []any{imap.SeenFlag}, nil)
}

func recipients(env *imap.Envelope, fallbackTo string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	add := func(addr string) {
		addr = strings.ToLower(strings.TrimSpace(addr))
		if addr == "" || seen[addr] {
			return
		}
		seen[addr] = true
		out = append(out, addr)
	}
	for _, a := range env.To {
		add(address(a))
	}
	for _, a := range env.Cc {
		add(address(a))
	}
	add(fallbackTo)
	return out
}

func address(a *imap.Address) string {
	if a == nil {
		return ""
	}
	if a.MailboxName == "" || a.HostName == "" {
		return strings.TrimSpace(a.MailboxName + a.HostName)
	}
	return a.MailboxName + "@" + a.HostName
}

func cfgHost(user string) string {
	if i := strings.LastIndex(user, "@"); i >= 0 {
		return user[i+1:]
	}
	return "mailpoll"
}

func loadConfig() config {
	c := config{
		host:        env("IMAP_HOST", "imap.gmail.com:993"),
		user:        os.Getenv("IMAP_USER"),
		pass:        os.Getenv("IMAP_PASSWORD"),
		mailbox:     env("IMAP_MAILBOX", "INBOX"),
		inboundURL:  env("INBOUND_URL", "http://go-api:8080/v1/inbound/email"),
		secret:      os.Getenv("LARAVEL_WEBHOOK_SECRET"),
		interval:    envDuration("MAILPOLL_INTERVAL", 30*time.Second),
		max:         envInt("MAILPOLL_MAX", 0),
		once:        os.Getenv("MAILPOLL_ONCE") == "1",
		statePath:   env("MAILPOLL_STATE", "tmp/mailpoll-state.json"),
		maxAttempts: envInt("MAILPOLL_MAX_ATTEMPTS", 10),
	}
	if c.maxAttempts < 1 {
		c.maxAttempts = 1
	}
	if c.user == "" || c.pass == "" {
		log.Fatal("mailpoll: IMAP_USER and IMAP_PASSWORD are required")
	}
	if c.secret == "" {
		log.Fatal("mailpoll: LARAVEL_WEBHOOK_SECRET is required")
	}
	return c
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
