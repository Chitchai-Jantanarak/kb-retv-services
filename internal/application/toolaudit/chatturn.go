package toolaudit

import (
	"context"
	"fmt"

	chatwf "github.com/my/app/internal/application/workflows/chat"
	"github.com/my/app/internal/domain/ports"
)

type ChatTurnAuditor struct {
	rec        Recorder
	agentIDFor AgentIDLookup
}

func NewChatTurn(rec Recorder, agentIDFor func(ctx context.Context, companyID int64) (int64, bool)) *ChatTurnAuditor {
	return &ChatTurnAuditor{rec: rec, agentIDFor: agentIDFor}
}

func (a *ChatTurnAuditor) RecordChatTurn(ctx context.Context, companyID int64, audit chatwf.TurnAudit) error {
	agentID, ok := a.agentIDFor(ctx, companyID)
	if !ok {
		return fmt.Errorf("toolaudit: no agent for company %d", companyID)
	}
	outcome := audit.Outcome
	if outcome == "" {
		outcome = "unknown"
	}
	_, err := a.rec.Record(ctx, ports.AIAction{
		CompanyID:  companyID,
		AgentID:    agentID,
		ActionType: "chat." + outcome,
		Input: map[string]any{
			"router_intent": audit.RouterIntent,
			"router_reason": audit.RouterReason,
			"resolved_from": audit.ResolvedFrom,
		},
		Output: map[string]any{
			"tool_id":    audit.ToolID,
			"local_only": audit.TokensIn == 0 && audit.TokensOut == 0,
		},
		TokensIn:   audit.TokensIn,
		TokensOut:  audit.TokensOut,
		LatencyMs:  audit.LatencyMS,
		Confidence: audit.Confidence,
	})
	return err
}

var _ chatwf.TurnRecorder = (*ChatTurnAuditor)(nil)
