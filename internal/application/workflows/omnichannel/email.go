package omnichannel

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/application/dto"
)

type emailPayload struct {
	MessageID  string   `json:"message_id"`
	InReplyTo  string   `json:"in_reply_to"`
	From       string   `json:"from"`
	To         string   `json:"to"`
	Recipients []string `json:"recipients"`
	Subject    string   `json:"subject"`
	Body       string   `json:"body"`
	BodyHTML   string   `json:"body_html"`
}

type EmailNormalizer struct{}

func (EmailNormalizer) Channel() string { return ChannelEmail }

func (n EmailNormalizer) Normalize(raw []byte) (Normalized, error) {
	var p emailPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return Normalized{}, fmt.Errorf("email: parse payload: %w", err)
	}
	sender := extractEmailAddress(p.From)
	if sender == "" {
		return Normalized{}, errors.New("email: payload has no From address")
	}
	if strings.TrimSpace(p.MessageID) == "" {
		return Normalized{}, errors.New("email: payload has no message_id")
	}
	body := strings.TrimSpace(p.Body)
	if body == "" {
		body = strings.TrimSpace(p.BodyHTML)
	}
	return Normalized{
		Request: dto.InboundMessageRequest{
			Channel:           ChannelEmail,
			ExternalMessageID: p.MessageID,
			CustomerID:        sender,
			Subject:           strings.TrimSpace(p.Subject),
			Body:              body,
		},
		ExternalSender:    sender,
		AccountExternalID: extractEmailAddress(p.To),
		AccountCandidates: emailCandidates(p),
		InReplyTo:         strings.TrimSpace(p.InReplyTo),
	}, nil
}

// emailCandidates lists every address the mail was addressed to, primary first.
// The configured mailbox among them is what authorizes intake; From never does.
func emailCandidates(p emailPayload) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(p.Recipients)+1)
	for _, raw := range append([]string{p.To}, p.Recipients...) {
		addr := strings.ToLower(extractEmailAddress(raw))
		if addr == "" || seen[addr] {
			continue
		}
		seen[addr] = true
		out = append(out, addr)
	}
	return out
}

func extractEmailAddress(from string) string {
	from = strings.TrimSpace(from)
	if from == "" {
		return ""
	}
	if i := strings.LastIndex(from, "<"); i >= 0 {
		if j := strings.Index(from[i:], ">"); j > 0 {
			return strings.TrimSpace(from[i+1 : i+j])
		}
	}
	return from
}
