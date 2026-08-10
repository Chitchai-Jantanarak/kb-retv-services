package mailpoll

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
)

type EmailPayload struct {
	MessageID       string       `json:"message_id"`
	InReplyTo       string       `json:"in_reply_to"`
	From            string       `json:"from"`
	To              string       `json:"to"`
	Recipients      []string     `json:"recipients"`
	Subject         string       `json:"subject"`
	Body            string       `json:"body"`
	BodyHTML        string       `json:"body_html"`
	References      []string     `json:"references"`
	FromName        string       `json:"from_name"`
	Date            string       `json:"date"`
	AutoSubmitted   string       `json:"auto_submitted"`
	ListUnsubscribe bool         `json:"list_unsubscribe"`
	Precedence      string       `json:"precedence"`
	Attachments     []Attachment `json:"attachments"`
}

type Attachment struct {
	Filename   string `json:"filename"`
	MIMEType   string `json:"mime_type"`
	SizeBytes  int    `json:"size_bytes"`
	ContentB64 string `json:"content_b64"`
}

type Forwarder struct {
	url    string
	secret string
	client *http.Client
}

func NewForwarder(url, secret string, client *http.Client) *Forwarder {
	if client == nil {
		client = http.DefaultClient
	}
	return &Forwarder{url: url, secret: secret, client: client}
}

func (f *Forwarder) Forward(ctx context.Context, p EmailPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AI-Signature", "sha256="+Sign(f.secret, body))

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("inbound returned status %d", resp.StatusCode)
	}
	return nil
}

func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
