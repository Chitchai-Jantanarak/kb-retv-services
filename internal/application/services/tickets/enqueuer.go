// Package tickets bridges the omnichannel workflow to the asynq queue:
// inbound messages produce a `ticket:create` job that the worker hands
// off to Laravel.
package tickets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/workflows/omnichannel"
	"github.com/my/app/internal/domain/ports"
)

const TaskCreate = "ticket:create"

type Enqueuer struct {
	queue ports.Queue
}

func NewEnqueuer(queue ports.Queue) (*Enqueuer, error) {
	if queue == nil {
		return nil, errors.New("tickets: queue is required")
	}
	return &Enqueuer{queue: queue}, nil
}

type Payload struct {
	CompanyID         int64  `json:"company_id"`
	ConversationID    int64  `json:"conversation_id"`
	MessageID         int64  `json:"message_id"`
	Channel           string `json:"channel"`
	ExternalMessageID string `json:"external_message_id"`
	CustomerID        string `json:"customer_id,omitempty"`
	SenderExternal    string `json:"sender_external,omitempty"`
	SenderName        string `json:"sender_name,omitempty"`
	Subject           string `json:"subject,omitempty"`
	Body              string `json:"body,omitempty"`
}

func (p Payload) IdempotencyKey() string {
	return fmt.Sprintf("ai:%s:%d:%d", p.Channel, p.ConversationID, p.MessageID)
}

func (e *Enqueuer) EnqueueTicket(ctx context.Context, companyID, conversationID, messageID int64, senderExternal string, req dto.InboundMessageRequest) error {
	payload, err := json.Marshal(Payload{
		CompanyID:         companyID,
		ConversationID:    conversationID,
		MessageID:         messageID,
		Channel:           req.Channel,
		ExternalMessageID: req.ExternalMessageID,
		CustomerID:        req.CustomerID,
		SenderExternal:    senderExternal,
		Subject:           req.Subject,
		Body:              req.Body,
	})
	if err != nil {
		return fmt.Errorf("tickets: marshal payload: %w", err)
	}
	return e.queue.Enqueue(ctx, ports.Job{
		Type:      TaskCreate,
		CompanyID: companyID,
		Payload:   payload,
	})
}

var _ omnichannel.TicketEnqueuer = (*Enqueuer)(nil)
