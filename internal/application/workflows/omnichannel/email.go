package omnichannel

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/my/app/internal/application/dto"
)

// routingKeyRE matches a mail binding's receiver address: either the
// plus-tag form on the intake mailbox (chitchai5757+sjt-8f31a2@gmail.com)
// or the alias-domain form (sjt-8f31a2@grubgrob.xyz), whose catch-all lands
// in the same mailbox. The key alphabet is [a-z0-9], 6-12 chars; matching
// runs against an already-lowercased bare address.
var routingKeyRE = regexp.MustCompile(`(?:^|\+)sjt-([a-z0-9]{6,12})@`)

type emailAttachment struct {
	Filename   string `json:"filename"`
	MIMEType   string `json:"mime_type"`
	SizeBytes  int    `json:"size_bytes"`
	ContentB64 string `json:"content_b64"`
}

type emailPayload struct {
	MessageID       string            `json:"message_id"`
	InReplyTo       string            `json:"in_reply_to"`
	From            string            `json:"from"`
	To              string            `json:"to"`
	Recipients      []string          `json:"recipients"`
	Subject         string            `json:"subject"`
	Body            string            `json:"body"`
	BodyHTML        string            `json:"body_html"`
	References      []string          `json:"references"`
	FromName        string            `json:"from_name"`
	Date            string            `json:"date"`
	AutoSubmitted   string            `json:"auto_submitted"`
	ListUnsubscribe bool              `json:"list_unsubscribe"`
	Precedence      string            `json:"precedence"`
	DeliveredTo     []string          `json:"delivered_to"`
	Attachments     []emailAttachment `json:"attachments"`
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
		ExternalSender:      sender,
		AccountExternalID:   extractEmailAddress(p.To),
		AccountCandidates:   emailCandidates(p),
		RoutingKeys:         routingKeysFrom(append(append([]string{p.To}, p.Recipients...), p.DeliveredTo...)...),
		InReplyTo:           strings.TrimSpace(p.InReplyTo),
		References:          trimmedNonEmpty(p.References),
		SenderName:          strings.TrimSpace(p.FromName),
		AutoSubmitted:       strings.TrimSpace(p.AutoSubmitted),
		ListUnsubscribe:     p.ListUnsubscribe,
		Precedence:          strings.TrimSpace(p.Precedence),
		AttachmentCount:     len(p.Attachments),
		AttachmentMIMETypes: attachmentMIMETypes(p.Attachments),
		Attachments:         decodeAttachments(p.Attachments),
	}, nil
}

func decodeAttachments(attachments []emailAttachment) []InboundAttachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]InboundAttachment, 0, len(attachments))
	for _, a := range attachments {
		data, err := base64.StdEncoding.DecodeString(a.ContentB64)
		if err != nil {
			continue
		}
		out = append(out, InboundAttachment{
			Filename: strings.TrimSpace(a.Filename),
			MIMEType: strings.TrimSpace(a.MIMEType),
			Data:     data,
		})
	}
	return out
}

func attachmentMIMETypes(attachments []emailAttachment) []string {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]string, 0, len(attachments))
	for _, a := range attachments {
		out = append(out, strings.TrimSpace(a.MIMEType))
	}
	return out
}

func trimmedNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// emailCandidates lists every address the mail was addressed to, primary
// first (To, then Recipients, then Delivered-To/X-Forwarded-To). The
// configured mailbox, an alias, or a routing key among them is what
// authorizes intake; From never does.
func emailCandidates(p emailPayload) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(p.Recipients)+len(p.DeliveredTo)+1)
	for _, raw := range append(append([]string{p.To}, p.Recipients...), p.DeliveredTo...) {
		addr := strings.ToLower(extractEmailAddress(raw))
		if addr == "" || seen[addr] {
			continue
		}
		seen[addr] = true
		out = append(out, addr)
	}
	return out
}

// routingKeysFrom extracts every +sjt-<key> routing tag found across the
// given raw address strings (which may carry a display name), in the order
// found, de-duplicated.
func routingKeysFrom(addrs ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range addrs {
		addr := strings.ToLower(extractEmailAddress(raw))
		if addr == "" {
			continue
		}
		m := routingKeyRE.FindStringSubmatch(addr)
		if m == nil {
			continue
		}
		key := m[1]
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
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
