package omnichannel

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strings"

	"github.com/my/app/internal/application/dto"
)

type lineWebhook struct {
	Destination string      `json:"destination"`
	Events      []lineEvent `json:"events"`
}

type lineEvent struct {
	Type            string       `json:"type"`
	Timestamp       int64        `json:"timestamp"`
	Source          lineSource   `json:"source"`
	Message         lineMessage  `json:"message"`
	ReplyToken      string       `json:"replyToken"`
	WebhookEventID  string       `json:"webhookEventId"`
	DeliveryContext lineDelivery `json:"deliveryContext"`
	Postback        linePostback `json:"postback"`
}

type lineDelivery struct {
	IsRedelivery bool `json:"isRedelivery"`
}

type linePostback struct {
	Data string `json:"data"`
}

type lineSource struct {
	Type    string `json:"type"`
	UserID  string `json:"userId"`
	GroupID string `json:"groupId"`
	RoomID  string `json:"roomId"`
}

type lineMessage struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Text     string `json:"text"`
	FileName string `json:"fileName"`
}

type LineNormalizer struct{}

func (LineNormalizer) Channel() string { return ChannelLine }

func (n LineNormalizer) Normalize(raw []byte) ([]Normalized, error) {
	var w lineWebhook
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, fmt.Errorf("line: parse webhook: %w", err)
	}
	if len(w.Events) == 0 {
		return nil, errors.New("line: webhook has no events")
	}

	out := make([]Normalized, 0, len(w.Events))
	var skipErr error
	for _, ev := range w.Events {
		sender := strings.TrimSpace(firstNonEmpty(ev.Source.UserID, ev.Source.GroupID, ev.Source.RoomID))
		if sender == "" {
			skipErr = errors.New("line: event has no source identifier")
			continue
		}
		// A webhook event with no "type" field predates this normalizer's
		// event-type routing (older fixtures, and any sender that never
		// adopted it); treat it as a message, matching prior behavior.
		eventType := ev.Type
		if eventType == "" {
			eventType = "message"
		}
		base := Normalized{
			ExternalSender:    sender,
			AccountExternalID: strings.TrimSpace(w.Destination),
			EventType:         eventType,
			ReplyToken:        ev.ReplyToken,
			IsRedelivery:      ev.DeliveryContext.IsRedelivery,
		}
		switch eventType {
		case "message":
			if strings.TrimSpace(ev.Message.ID) == "" {
				skipErr = errors.New("line: event has no message id")
				continue
			}
			base.Request = messageRequest(ev, sender)
		case "follow", "unfollow":
			base.Request = dto.InboundMessageRequest{
				Channel:           ChannelLine,
				ExternalMessageID: ev.WebhookEventID,
				CustomerID:        sender,
			}
		case "postback":
			base.PostbackData = ev.Postback.Data
			base.Request = dto.InboundMessageRequest{
				Channel:           ChannelLine,
				ExternalMessageID: ev.WebhookEventID,
				CustomerID:        sender,
			}
		default:
			continue
		}
		out = append(out, base)
	}
	if len(out) == 0 && skipErr != nil {
		return nil, skipErr
	}
	return out, nil
}

func messageRequest(ev lineEvent, sender string) dto.InboundMessageRequest {
	body := ev.Message.Text
	var attachments []dto.AttachmentRef
	switch ev.Message.Type {
	case "image":
		body = ""
		attachments = []dto.AttachmentRef{{ID: ev.Message.ID, MIMEType: "image/jpeg"}}
	case "audio":
		body = ""
		attachments = []dto.AttachmentRef{{ID: ev.Message.ID, MIMEType: "audio/m4a"}}
	case "video":
		body = ""
		attachments = []dto.AttachmentRef{{ID: ev.Message.ID, MIMEType: "video/mp4"}}
	case "file":
		body = ev.Message.FileName
		attachments = []dto.AttachmentRef{{ID: ev.Message.ID, MIMEType: mimeForFileName(ev.Message.FileName)}}
	}
	return dto.InboundMessageRequest{
		Channel:           ChannelLine,
		ExternalMessageID: ev.Message.ID,
		CustomerID:        sender,
		Body:              body,
		Attachments:       attachments,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func mimeForFileName(name string) string {
	if m := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); m != "" {
		if i := strings.IndexByte(m, ';'); i > 0 {
			m = m[:i]
		}
		return m
	}
	return "application/octet-stream"
}
