package omnichannel

import (
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/application/dto"
)

const (
	ChannelLine  = "line"
	ChannelEmail = "email"
	ChannelWeb   = "web"
	ChannelAPI   = "api"
)

const ActionIntakeAssessed = "ai_intake_assessed"
const ActionTicketEnqueueFailed = "ai_ticket_enqueue_failed"

type InboundAttachment struct {
	Filename string
	MIMEType string
	Data     []byte
}

type Normalized struct {
	Request             dto.InboundMessageRequest
	ExternalSender      string
	AccountExternalID   string
	AccountCandidates   []string
	InReplyTo           string
	References          []string
	SenderName          string
	AutoSubmitted       string
	ListUnsubscribe     bool
	Precedence          string
	AttachmentCount     int
	AttachmentMIMETypes []string
	Attachments         []InboundAttachment
}

type Normalizer interface {
	Channel() string
	Normalize(raw []byte) (Normalized, error)
}

type NormalizerRegistry struct {
	byChannel map[string]Normalizer
}

func NewNormalizerRegistry(normalizers ...Normalizer) (*NormalizerRegistry, error) {
	r := &NormalizerRegistry{byChannel: make(map[string]Normalizer, len(normalizers))}
	for _, n := range normalizers {
		if n == nil {
			continue
		}
		ch := strings.ToLower(strings.TrimSpace(n.Channel()))
		if ch == "" {
			return nil, errors.New("omnichannel: normalizer with empty channel")
		}
		if _, dup := r.byChannel[ch]; dup {
			return nil, fmt.Errorf("omnichannel: duplicate normalizer for channel %q", ch)
		}
		r.byChannel[ch] = n
	}
	return r, nil
}

func (r *NormalizerRegistry) Get(channel string) (Normalizer, error) {
	ch := strings.ToLower(strings.TrimSpace(channel))
	n, ok := r.byChannel[ch]
	if !ok {
		return nil, fmt.Errorf("omnichannel: no normalizer for channel %q", channel)
	}
	return n, nil
}

func (r *NormalizerRegistry) Channels() []string {
	out := make([]string, 0, len(r.byChannel))
	for ch := range r.byChannel {
		out = append(out, ch)
	}
	return out
}
