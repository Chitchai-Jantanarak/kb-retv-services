package omnichannel

import (
	"encoding/json"
	"testing"

	"github.com/my/app/internal/mailpoll"
)

func TestEmailPayloadRoundTripKeepsNewFields(t *testing.T) {
	forwarderPayload := mailpoll.EmailPayload{
		MessageID:       "<msg-1@example.com>",
		InReplyTo:       "<msg-0@example.com>",
		From:            "Jane Doe <jane@example.com>",
		To:              "support@example.com",
		Recipients:      []string{"support@example.com", "cc@example.com"},
		Subject:         "help needed",
		Body:            "plain body",
		BodyHTML:        "<p>plain body</p>",
		References:      []string{"<msg-a@example.com>", "<msg-b@example.com>"},
		FromName:        "Jane Doe",
		Date:            "2026-08-10T12:00:00Z",
		AutoSubmitted:   "auto-replied",
		ListUnsubscribe: true,
		Precedence:      "bulk",
		Attachments: []mailpoll.Attachment{
			{
				Filename:   "photo.jpg",
				MIMEType:   "image/jpeg",
				SizeBytes:  4,
				ContentB64: "AAECAw==",
			},
		},
	}

	raw, err := json.Marshal(forwarderPayload)
	if err != nil {
		t.Fatalf("marshal forwarder payload: %v", err)
	}

	var decoded emailPayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal into omnichannel emailPayload: %v", err)
	}

	if decoded.MessageID != forwarderPayload.MessageID {
		t.Errorf("MessageID: got %q want %q", decoded.MessageID, forwarderPayload.MessageID)
	}
	if decoded.InReplyTo != forwarderPayload.InReplyTo {
		t.Errorf("InReplyTo: got %q want %q", decoded.InReplyTo, forwarderPayload.InReplyTo)
	}
	if decoded.From != forwarderPayload.From {
		t.Errorf("From: got %q want %q", decoded.From, forwarderPayload.From)
	}
	if decoded.To != forwarderPayload.To {
		t.Errorf("To: got %q want %q", decoded.To, forwarderPayload.To)
	}
	if len(decoded.Recipients) != len(forwarderPayload.Recipients) {
		t.Errorf("Recipients: got %v want %v", decoded.Recipients, forwarderPayload.Recipients)
	}
	if decoded.Subject != forwarderPayload.Subject {
		t.Errorf("Subject: got %q want %q", decoded.Subject, forwarderPayload.Subject)
	}
	if decoded.Body != forwarderPayload.Body {
		t.Errorf("Body: got %q want %q", decoded.Body, forwarderPayload.Body)
	}
	if decoded.BodyHTML != forwarderPayload.BodyHTML {
		t.Errorf("BodyHTML: got %q want %q", decoded.BodyHTML, forwarderPayload.BodyHTML)
	}
	if len(decoded.References) != len(forwarderPayload.References) {
		t.Errorf("References: got %v want %v", decoded.References, forwarderPayload.References)
	}
	if decoded.FromName != forwarderPayload.FromName {
		t.Errorf("FromName: got %q want %q", decoded.FromName, forwarderPayload.FromName)
	}
	if decoded.Date != forwarderPayload.Date {
		t.Errorf("Date: got %q want %q", decoded.Date, forwarderPayload.Date)
	}
	if decoded.AutoSubmitted != forwarderPayload.AutoSubmitted {
		t.Errorf("AutoSubmitted: got %q want %q", decoded.AutoSubmitted, forwarderPayload.AutoSubmitted)
	}
	if decoded.ListUnsubscribe != forwarderPayload.ListUnsubscribe {
		t.Errorf("ListUnsubscribe: got %v want %v", decoded.ListUnsubscribe, forwarderPayload.ListUnsubscribe)
	}
	if decoded.Precedence != forwarderPayload.Precedence {
		t.Errorf("Precedence: got %q want %q", decoded.Precedence, forwarderPayload.Precedence)
	}
	if len(decoded.Attachments) != 1 {
		t.Fatalf("Attachments: expected 1, got %d", len(decoded.Attachments))
	}
	got := decoded.Attachments[0]
	want := forwarderPayload.Attachments[0]
	if got.Filename != want.Filename || got.MIMEType != want.MIMEType || got.SizeBytes != want.SizeBytes || got.ContentB64 != want.ContentB64 {
		t.Errorf("Attachments[0]: got %+v want %+v", got, want)
	}
}
