package activityaudit

import (
	"context"

	"go.uber.org/zap"

	"github.com/my/app/internal/domain/ports"
)

type AIAction struct {
	inner    ports.AIActionRecorder
	activity ports.ActivityRecorder
	log      *zap.Logger
}

func NewAIAction(inner ports.AIActionRecorder, activity ports.ActivityRecorder, log *zap.Logger) *AIAction {
	if log == nil {
		log = zap.NewNop()
	}
	return &AIAction{inner: inner, activity: activity, log: log}
}

func (a *AIAction) Record(ctx context.Context, action ports.AIAction) (int64, error) {
	id, err := a.inner.Record(ctx, action)
	if err != nil {
		return id, err
	}
	if a.activity == nil {
		return id, nil
	}

	entry := ports.ActivityEntry{
		CompanyID:   action.CompanyID,
		ActorType:   ports.ActorTypeAIAgent,
		ActorID:     action.AgentID,
		SubjectType: "ai_action",
		SubjectID:   id,
		Action:      ports.ActivityCreate,
		Context: map[string]any{
			"action_type": action.ActionType,
			"latency_ms":  action.LatencyMs,
		},
	}

	if aerr := a.activity.RecordActivity(ctx, entry); aerr != nil {
		a.log.Warn("activityaudit: ai action activity record failed",
			zap.Int64("company_id", action.CompanyID),
			zap.Int64("ai_agent_id", action.AgentID),
			zap.Int64("ai_action_id", id),
			zap.Error(aerr))
	}

	return id, nil
}

var _ ports.AIActionRecorder = (*AIAction)(nil)
