package chat

import (
	"context"

	"go.uber.org/zap"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/intent"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/shared/logger"
)

type TurnAudit struct {
	Outcome      string
	RouterIntent string
	RouterReason string
	Confidence   float64
	ToolID       string
	ResolvedFrom string
	LatencyMS    int
	TokensIn     int
	TokensOut    int
}

type TurnRecorder interface {
	RecordChatTurn(ctx context.Context, companyID int64, audit TurnAudit) error
}

func toolIDOf(r *skeleton.Response) string {
	if r == nil {
		return ""
	}
	return r.ToolID
}

func paramsOf(r *skeleton.Response) map[string]string {
	if r == nil {
		return nil
	}
	return r.Params
}

func resolvedFrom(params map[string]string, candidates []string) string {
	if len(params) == 0 {
		return "none"
	}
	for _, v := range params {
		for _, c := range candidates {
			if v == c {
				return "reference"
			}
		}
	}
	return "text"
}

func (w *Workflow) recordTurn(ctx context.Context, companyID int64, resp dto.ChatResponse, decision intent.Decision, audit TurnAudit) {
	if w.turns == nil {
		return
	}
	audit.Outcome = resp.Status
	audit.RouterIntent = string(decision.Intent)
	audit.RouterReason = string(decision.Reason)
	audit.Confidence = decision.Confidence
	if err := w.turns.RecordChatTurn(ctx, companyID, audit); err != nil {
		logger.FromContext(ctx).Warn("chat: record turn", zap.Error(err))
	}
}
