package toolaudit

import (
	"context"

	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/domain/ports"
)

type Recorder interface {
	Record(ctx context.Context, action ports.AIAction) (int64, error)
}

type Auditor struct {
	rec Recorder
}

func New(rec Recorder) *Auditor {
	return &Auditor{rec: rec}
}

func (a *Auditor) Log(ctx context.Context, tag string, actor skeleton.Actor, toolID string) error {
	_, err := a.rec.Record(ctx, ports.AIAction{
		CompanyID:  actor.CompanyID,
		ActionType: tag,
		Input:      toolID,
	})
	return err
}
