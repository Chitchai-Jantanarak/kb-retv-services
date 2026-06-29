package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"

	"github.com/my/app/internal/mailpoll"
)

type config struct {
	host, user, pass, mailbox string
	inboundURL, secret        string
	interval                  time.Duration
	max                       int
	once                      bool
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

	if _, err := c.Select(cfg.mailbox, false); err != nil {
		return fmt.Errorf("select %q: %w", cfg.mailbox, err)
	}

	criteria := imap.NewSearchCriteria()
	criteria.WithoutFlags = []string{imap.SeenFlag}
	uids, err := c.UidSearch(criteria)
	if err != nil {
		return fmt.Errorf("search unseen: %w", err)
	}
	if len(uids) == 0 {
		return nil
	}
	if cfg.max > 0 && len(uids) > cfg.max {
		uids = uids[:cfg.max]
	}
	log.Printf("mailpoll: %d new message(s) to ingest", len(uids))

	section := &imap.BodySectionName{}
	for _, uid := range uids {
		if err := ctx.Err(); err != nil {
			return err
		}
		p, err := fetchPayload(c, uid, section, cfg.user)
		if err != nil {
			log.Printf("mailpoll: uid %d fetch: %v", uid, err)
			continue
		}
		if err := fwd.Forward(ctx, p); err != nil {
			log.Printf("mailpoll: uid %d forward: %v", uid, err)
			continue
		}
		markSeen(c, uid)
		log.Printf("mailpoll: ingested uid=%d from=%s subject=%q", uid, p.From, p.Subject)
	}
	return nil
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

	p := mailpoll.EmailPayload{To: fallbackTo}
	if msg.Envelope != nil {
		p.MessageID = msg.Envelope.MessageId
		p.Subject = msg.Envelope.Subject
		if len(msg.Envelope.From) > 0 {
			p.From = address(msg.Envelope.From[0])
		}
		if len(msg.Envelope.To) > 0 {
			p.To = address(msg.Envelope.To[0])
		}
	}
	if strings.TrimSpace(p.MessageID) == "" {
		p.MessageID = fmt.Sprintf("<mailpoll-%d@%s>", uid, cfgHost(fallbackTo))
	}

	if body := msg.GetBody(section); body != nil {
		raw, _ := io.ReadAll(body)
		p.Body, p.BodyHTML = extractBody(raw)
	}
	return p, nil
}

func extractBody(raw []byte) (string, string) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return strings.TrimSpace(string(raw)), ""
	}
	var text, html string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		h, ok := part.Header.(*mail.InlineHeader)
		if !ok {
			continue
		}
		b, _ := io.ReadAll(part.Body)
		ct, _, _ := h.ContentType()
		if strings.Contains(ct, "html") {
			html = string(b)
		} else {
			text = string(b)
		}
	}
	return strings.TrimSpace(text), strings.TrimSpace(html)
}

func markSeen(c *client.Client, uid uint32) {
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	item := imap.FormatFlagsOp(imap.AddFlags, true)
	_ = c.UidStore(seqset, item, []interface{}{imap.SeenFlag}, nil)
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
		host:       env("IMAP_HOST", "imap.gmail.com:993"),
		user:       os.Getenv("IMAP_USER"),
		pass:       os.Getenv("IMAP_PASSWORD"),
		mailbox:    env("IMAP_MAILBOX", "INBOX"),
		inboundURL: env("INBOUND_URL", "http://go-api:8080/v1/inbound/email"),
		secret:     os.Getenv("LARAVEL_WEBHOOK_SECRET"),
		interval:   envDuration("MAILPOLL_INTERVAL", 30*time.Second),
		max:        envInt("MAILPOLL_MAX", 0),
		once:       os.Getenv("MAILPOLL_ONCE") == "1",
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
