package toolaudit

import (
	"context"
	"fmt"

	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/domain/ports"
)

type Recorder interface {
	Record(ctx context.Context, action ports.AIAction) (int64, error)
}

type AgentIDLookup func(ctx context.Context, companyID int64) (int64, bool)

type Auditor struct {
	rec        Recorder
	agentIDFor AgentIDLookup
}

func New(rec Recorder, agentIDFor func(ctx context.Context, companyID int64) (int64, bool)) *Auditor {
	return &Auditor{rec: rec, agentIDFor: agentIDFor}
}

func (a *Auditor) Log(ctx context.Context, tag string, actor skeleton.Actor, toolID string) error {
	agentID, ok := a.agentIDFor(ctx, actor.CompanyID)
	if !ok {
		return fmt.Errorf("toolaudit: no agent for company %d", actor.CompanyID)
	}
	_, err := a.rec.Record(ctx, ports.AIAction{
		CompanyID:  actor.CompanyID,
		AgentID:    agentID,
		ActionType: tag,
		Input:      toolID,
	})
	return err
}
