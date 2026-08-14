// Package intakeassess bridges the omnichannel workflow to the asynq queue:
// inbound email produces an `intake:assess` job so the tier1 LLM/vision
// assessment runs in the worker, off the mailpoll HTTP deadline.
package intakeassess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/workflows/omnichannel"
	"github.com/my/app/internal/domain/ports"
)

const TaskAssess = "intake:assess"

const assessMaxRetry = 5

type Enqueuer struct {
	queue ports.Queue
}

func NewEnqueuer(queue ports.Queue) (*Enqueuer, error) {
	if queue == nil {
		return nil, errors.New("intakeassess: queue is required")
	}
	return &Enqueuer{queue: queue}, nil
}

type Payload struct {
	ConversationID  int64                     `json:"conversation_id"`
	MessageID       int64                     `json:"message_id"`
	Customer        string                    `json:"customer"`
	ListUnsubscribe bool                      `json:"list_unsubscribe,omitempty"`
	AutoSubmitted   string                    `json:"auto_submitted,omitempty"`
	Precedence      string                    `json:"precedence,omitempty"`
	HasAttachments  bool                      `json:"has_attachments,omitempty"`
	ReferencedCase  string                    `json:"referenced_case,omitempty"`
	ThreadMatched   bool                      `json:"thread_matched,omitempty"`
	Images          []ports.PromptImage       `json:"images,omitempty"`
	Request         dto.InboundMessageRequest `json:"request"`
}

func (e *Enqueuer) EnqueueAssess(ctx context.Context, companyID, conversationID, messageID int64, customer string, sig omnichannel.IntakeSignals, req dto.InboundMessageRequest) error {
	payload, err := json.Marshal(Payload{
		ConversationID:  conversationID,
		MessageID:       messageID,
		Customer:        customer,
		ListUnsubscribe: sig.ListUnsubscribe,
		AutoSubmitted:   sig.AutoSubmitted,
		Precedence:      sig.Precedence,
		HasAttachments:  sig.HasAttachments,
		ReferencedCase:  sig.ReferencedCase,
		ThreadMatched:   sig.ThreadMatched,
		Images:          sig.Images,
		Request:         req,
	})
	if err != nil {
		return fmt.Errorf("intakeassess: marshal payload: %w", err)
	}
	return e.queue.Enqueue(ctx, ports.Job{
		Type:      TaskAssess,
		CompanyID: companyID,
		Payload:   payload,
		Attempt:   assessMaxRetry,
	})
}

var _ omnichannel.AssessEnqueuer = (*Enqueuer)(nil)
