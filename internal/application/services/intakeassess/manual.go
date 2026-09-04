package intakeassess

import (
	"context"
	"errors"

	"github.com/my/app/internal/application/workflows/omnichannel"
)

type Manual struct {
	loader omnichannel.AssessmentDraftLoader
	queue  omnichannel.AssessEnqueuer
}

func NewManual(loader omnichannel.AssessmentDraftLoader, queue omnichannel.AssessEnqueuer) (*Manual, error) {
	if loader == nil {
		return nil, errors.New("intakeassess: draft loader is required")
	}
	if queue == nil {
		return nil, errors.New("intakeassess: assess queue is required")
	}
	return &Manual{loader: loader, queue: queue}, nil
}

func (m *Manual) EnqueueDraft(ctx context.Context, companyID, conversationID int64) (int64, error) {
	draft, err := m.loader.LoadAssessmentDraft(ctx, companyID, conversationID)
	if err != nil {
		return 0, err
	}
	if err := m.queue.EnqueueAssess(
		ctx,
		draft.CompanyID,
		draft.ConversationID,
		draft.MessageID,
		draft.Customer,
		draft.Signals,
		draft.Request,
	); err != nil {
		return 0, err
	}
	return draft.MessageID, nil
}
