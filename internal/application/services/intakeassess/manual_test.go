package intakeassess

import (
	"context"
	"errors"
	"testing"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/workflows/omnichannel"
)

type stubDraftLoader struct {
	draft omnichannel.AssessmentDraft
	err   error
}

func (s stubDraftLoader) LoadAssessmentDraft(context.Context, int64, int64) (omnichannel.AssessmentDraft, error) {
	return s.draft, s.err
}

type captureAssessQueue struct {
	companyID      int64
	conversationID int64
	messageID      int64
	customer       string
	signals        omnichannel.IntakeSignals
	request        dto.InboundMessageRequest
}

func (q *captureAssessQueue) EnqueueAssess(_ context.Context, companyID, conversationID, messageID int64, customer string, sig omnichannel.IntakeSignals, req dto.InboundMessageRequest) error {
	q.companyID = companyID
	q.conversationID = conversationID
	q.messageID = messageID
	q.customer = customer
	q.signals = sig
	q.request = req
	return nil
}

func TestManualEnqueueDraftUsesStoredDraft(t *testing.T) {
	queue := &captureAssessQueue{}
	manual, err := NewManual(stubDraftLoader{draft: omnichannel.AssessmentDraft{
		CompanyID:      7,
		ConversationID: 11,
		MessageID:      13,
		Customer:       "customer@example.com",
		Signals:        omnichannel.IntakeSignals{Subject: "Printer stopped"},
		Request:        dto.InboundMessageRequest{Channel: "email", Body: "It stopped"},
	}}, queue)
	if err != nil {
		t.Fatalf("NewManual: %v", err)
	}

	messageID, err := manual.EnqueueDraft(context.Background(), 7, 11)
	if err != nil {
		t.Fatalf("EnqueueDraft: %v", err)
	}
	if messageID != 13 || queue.companyID != 7 || queue.conversationID != 11 || queue.messageID != 13 {
		t.Fatalf("queued IDs = company:%d conversation:%d message:%d, returned:%d", queue.companyID, queue.conversationID, queue.messageID, messageID)
	}
	if queue.customer != "customer@example.com" || queue.signals.Subject != "Printer stopped" || queue.request.Body != "It stopped" {
		t.Fatalf("queued draft = customer:%q signals:%+v request:%+v", queue.customer, queue.signals, queue.request)
	}
}

func TestManualEnqueueDraftReturnsLoaderError(t *testing.T) {
	want := errors.New("load failed")
	manual, err := NewManual(stubDraftLoader{err: want}, &captureAssessQueue{})
	if err != nil {
		t.Fatalf("NewManual: %v", err)
	}
	if _, err := manual.EnqueueDraft(context.Background(), 7, 11); !errors.Is(err, want) {
		t.Fatalf("EnqueueDraft error = %v, want %v", err, want)
	}
}
