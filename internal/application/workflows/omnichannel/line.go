package omnichannel

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/application/dto"
)

type lineWebhook struct {
	Destination string      `json:"destination"`
	Events      []lineEvent `json:"events"`
}

type lineEvent struct {
	Type       string      `json:"type"`
	Timestamp  int64       `json:"timestamp"`
	Source     lineSource  `json:"source"`
	Message    lineMessage `json:"message"`
	ReplyToken string      `json:"replyToken"`
}

type lineSource struct {
	Type    string `json:"type"`
	UserID  string `json:"userId"`
	GroupID string `json:"groupId"`
	RoomID  string `json:"roomId"`
}

type lineMessage struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Text string `json:"text"`
}

type LineNormalizer struct{}

func (LineNormalizer) Channel() string { return ChannelLine }

func (n LineNormalizer) Normalize(raw []byte) (Normalized, error) {
	var w lineWebhook
	if err := json.Unmarshal(raw, &w); err != nil {
		return Normalized{}, fmt.Errorf("line: parse webhook: %w", err)
	}
	if len(w.Events) == 0 {
		return Normalized{}, errors.New("line: webhook has no events")
	}
	ev := w.Events[0]
	sender := strings.TrimSpace(firstNonEmpty(ev.Source.UserID, ev.Source.GroupID, ev.Source.RoomID))
	if sender == "" {
		return Normalized{}, errors.New("line: event has no source identifier")
	}
	if strings.TrimSpace(ev.Message.ID) == "" {
		return Normalized{}, errors.New("line: event has no message id")
	}

	body := ev.Message.Text
	var attachments []dto.AttachmentRef
	switch ev.Message.Type {
	case "image":
		body = ""
		attachments = []dto.AttachmentRef{{ID: ev.Message.ID, MIMEType: "image/jpeg"}}
	case "audio":
		body = ""
		attachments = []dto.AttachmentRef{{ID: ev.Message.ID, MIMEType: "audio/m4a"}}
	}

	return Normalized{
		Request: dto.InboundMessageRequest{
			Channel:           ChannelLine,
			ExternalMessageID: ev.Message.ID,
			CustomerID:        sender,
			Body:              body,
			Attachments:       attachments,
		},
		ExternalSender:    sender,
		AccountExternalID: strings.TrimSpace(w.Destination),
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
