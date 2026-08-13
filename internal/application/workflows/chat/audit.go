package chat

import (
	"context"
	"slices"
	"time"

	"go.uber.org/zap"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/intent"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/shared/logger"
)

const audienceEmployee = "employee"

type TurnAudit struct {
	Outcome       string
	RouterIntent  string
	RouterReason  string
	Confidence    float64
	ToolID        string
	ResolvedFrom  string
	LatencyMS     int
	TokensIn      int
	TokensOut     int
	DecisionKind  string
	Score         float64
	RunnerUpScore float64
	Margin        float64
	RowsReturned  int
	Lang          string
	Audience      string
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

func scoreOf(r *skeleton.Response) float64 {
	if r == nil {
		return 0
	}
	return r.SelectionScore
}

func runnerUpOf(r *skeleton.Response) float64 {
	if r == nil {
		return 0
	}
	return r.RunnerUpScore
}

func rowsOf(r *skeleton.Response) int {
	if r == nil {
		return 0
	}
	return r.RowCount
}

func decisionKindOf(status string, decision intent.Decision, pre chatPreamble) string {
	if !pre.handled {
		return "generative"
	}
	if pre.tool != nil {
		return "tool"
	}
	switch status {
	case dto.ChatStatusHandoff:
		return "handoff"
	case dto.ChatStatusOffDomain:
		return "blocked"
	case dto.ChatStatusNeedsClarification:
		return "clarify"
	case dto.ChatStatusPermissionDenied, dto.ChatStatusToolFailed:
		return "no_tool"
	}
	if decision.Intent == intent.Social {
		return "no_tool"
	}
	return "generative"
}

func resolvedFrom(params map[string]string, candidates []string) string {
	if len(params) == 0 {
		return "none"
	}
	for _, v := range params {
		if slices.Contains(candidates, v) {
			return "reference"
		}
	}
	return "text"
}

func (w *Workflow) recordTurn(ctx context.Context, companyID int64, status string, decision intent.Decision, audit TurnAudit) {
	if w.turns == nil {
		return
	}
	audit.Outcome = status
	audit.RouterIntent = string(decision.Intent)
	audit.RouterReason = string(decision.Reason)
	audit.Confidence = decision.Confidence
	if err := w.turns.RecordChatTurn(ctx, companyID, audit); err != nil {
		logger.FromContext(ctx).Warn("chat: record turn", zap.Error(err))
	}
}

func (w *Workflow) finalizeTurn(ctx context.Context, status string, pre chatPreamble, started time.Time, tokensIn, tokensOut int, lang string) {
	score := scoreOf(pre.toolTrace)
	runnerUp := runnerUpOf(pre.toolTrace)
	w.recordTurn(ctx, pre.companyID, status, pre.decision, TurnAudit{
		ToolID:        toolIDOf(pre.tool),
		ResolvedFrom:  resolvedFrom(paramsOf(pre.tool), pre.candidates),
		LatencyMS:     int(time.Since(started).Milliseconds()),
		TokensIn:      tokensIn,
		TokensOut:     tokensOut,
		DecisionKind:  decisionKindOf(status, pre.decision, pre),
		Score:         score,
		RunnerUpScore: runnerUp,
		Margin:        score - runnerUp,
		RowsReturned:  rowsOf(pre.toolTrace),
		Lang:          lang,
		Audience:      audienceEmployee,
	})
}
