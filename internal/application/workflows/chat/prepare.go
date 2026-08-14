package chat

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/my/app/internal/application/decide"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/intent"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/application/tools"
	"github.com/my/app/internal/domain/ports"
	chatsession "github.com/my/app/internal/repositories/chatsession/mysql"
	"github.com/my/app/internal/shared/ctxkey"
	"github.com/my/app/internal/shared/debugtrace"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/shared/logger"
)

type chatPreamble struct {
	companyID  int64
	lastUser   string
	resp       dto.ChatResponse
	handled    bool
	toolDebug  *dto.ChatToolDebug
	transcript string
	tool       *skeleton.Response
	toolTrace  *skeleton.Response
	decision   intent.Decision
	candidates []string
}

// prepare runs the pre-stages shared by Run and RunStream: request scope
// checks, audio transcription, intent routing, and the tool orchestrator
// short-circuit. It returns handled=true when the caller should use resp as
// the final answer without touching the LLM.
func (w *Workflow) prepare(ctx context.Context, req dto.ChatRequest, timings map[string]int64) (chatPreamble, error) {
	companyID, ok := ctxkey.CompanyID(ctx)
	if !ok || companyID <= 0 {
		return chatPreamble{}, apperr.New(apperr.CodeUnauthorized, "company scope is required")
	}

	transcript, err := w.transcribeLastUserAudio(ctx, req)
	if err != nil {
		return chatPreamble{}, err
	}

	lastUser := req.LastUserMessage()
	if lastUser == "" {
		return chatPreamble{}, apperr.New(apperr.CodeInvalidInput, "a user message is required")
	}

	if w.router == nil {
		return chatPreamble{companyID: companyID, lastUser: lastUser, transcript: transcript}, nil
	}

	decider := decide.Decider{Router: w.router, OffTopicMargin: w.offTopicMargin}
	if w.orch != nil {
		decider.Runner = w.orch
	}
	decideCtx := ctx
	var traceCollector *debugtrace.Collector
	if req.Debug {
		decideCtx, traceCollector = debugtrace.WithCollector(ctx)
	}
	candidates := w.citeCandidates(ctx, companyID, req)

	outcome, err := decider.Decide(decideCtx, decide.Input{
		Actor:          toolActor(ctx, companyID),
		Text:           lastUser,
		CiteCandidates: candidates,
	}, func(stage string, fn func() error) error {
		return timedChatErr(timings, stage, fn)
	})
	if err != nil {
		return chatPreamble{}, err
	}
	decision := outcome.Decision

	var selectedToolDebug *dto.ChatToolDebug
	toolTrace := outcome.Tool
	if outcome.Tool != nil {
		if req.Debug {
			selectedToolDebug = buildToolDebug(*outcome.Tool, outcome.ToolErr, timings["tool"], traceCollector.Events())
			if selectedToolDebug != nil {
				selectedToolDebug.RouterIntent = string(decision.Intent)
				selectedToolDebug.RouterConfidence = decision.Confidence
				selectedToolDebug.RetrievalScore = decision.RetrievalScore
			}
		}
		if outcome.ToolErr != nil {
			logger.FromContext(ctx).Error("chat: tool orchestrator failed", zap.Int64("company_id", companyID), zap.Error(outcome.ToolErr))
		}
	}

	base := chatPreamble{
		companyID:  companyID,
		lastUser:   lastUser,
		toolDebug:  selectedToolDebug,
		transcript: transcript,
		toolTrace:  toolTrace,
		decision:   decision,
	}

	switch outcome.Kind {
	case decide.KindClarify:
		reply := clarifyReply(req.Locale)
		var params map[string]string
		if outcome.Tool != nil {
			params = outcome.Tool.Params
		}
		if modelReply, ok := w.clarifyViaModel(ctx, req.Locale, lastUser, companyID, outcome.Missing, params); ok {
			reply = modelReply
		}
		base.handled = true
		base.resp = dto.ChatResponse{
			Reply:    reply,
			Status:   dto.ChatStatusNeedsClarification,
			Activity: activity(req.Locale, "request_checked", "permission_checked"),
		}
		return base, nil
	case decide.KindToolFailed:
		base.handled = true
		base.resp = dto.ChatResponse{
			Reply:    toolFailedReply(req.Locale),
			Status:   dto.ChatStatusToolFailed,
			Activity: activity(req.Locale, "request_checked", "permission_checked"),
		}
		return base, nil
	case decide.KindPermissionDenied:
		base.handled = true
		base.resp = dto.ChatResponse{
			Reply:    permissionDeniedReply(req.Locale),
			Status:   dto.ChatStatusPermissionDenied,
			Activity: activity(req.Locale, "request_checked", "permission_checked"),
		}
		return base, nil
	case decide.KindTool:
		toolResp := *outcome.Tool
		resp := toolChatResponse(req.Locale, toolResp)
		if summaryComposeMode(toolResp, req) {
			timedChat(timings, "compose", func() {
				if summary, ok := w.composeToolReply(ctx, req.Locale, lastUser, companyID, toolResp); ok {
					resp.Reply = summary
				}
			})
		}
		base.handled = true
		base.resp = resp
		base.tool = outcome.Tool
		base.candidates = candidates
		return base, nil
	case decide.KindHandoff, decide.KindSocial, decide.KindOffTopic:
		seed := replySeed(companyID, lastUser, len(req.Messages))
		if resp, handled := w.shortCircuit(req.Locale, decision, outcome.Kind, seed); handled {
			base.handled = true
			base.resp = resp
		}
		return base, nil
	default:
		return base, nil
	}
}

// transcribeLastUserAudio finds the first audio attachment on the last user
// message, fetches and transcribes it, and mutates req.Messages in place
// with the effective text per the dumb rule: audio-only replaces content,
// audio+text appends the transcript as a quoted block. req.Messages shares
// its backing array with the caller, so this mutation is visible to Run and
// RunStream once prepare returns.
func (w *Workflow) transcribeLastUserAudio(ctx context.Context, req dto.ChatRequest) (string, error) {
	if w.transcriber == nil || w.fetcher == nil {
		return "", nil
	}

	idx := -1
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == dto.ChatRoleUser {
			idx = i
			break
		}
	}
	if idx < 0 {
		return "", nil
	}

	var audioAtt *dto.AttachmentRef
	for i := range req.Messages[idx].Attachments {
		if strings.HasPrefix(strings.ToLower(req.Messages[idx].Attachments[i].MIMEType), "audio/") {
			audioAtt = &req.Messages[idx].Attachments[i]
			break
		}
	}
	if audioAtt == nil {
		return "", nil
	}

	data, err := w.fetcher.Fetch(ctx, toPromptAttachment(*audioAtt))
	if err != nil {
		return "", err
	}
	result, err := w.transcriber.Transcribe(ctx, ports.AudioInput{
		Bytes:      data.Bytes,
		MIMEType:   data.MIMEType,
		LocaleHint: req.Locale,
	})
	if err != nil {
		return "", err
	}

	transcript := strings.TrimSpace(result.Text)
	if strings.TrimSpace(req.Messages[idx].Content) == "" {
		req.Messages[idx].Content = transcript
	} else {
		req.Messages[idx].Content = req.Messages[idx].Content + "\n\n> " + transcript
	}
	return transcript, nil
}

func priorAssistantTurns(messages []dto.ChatMessage) []string {
	out := make([]string, 0, len(messages))
	for _, m := range messages {
		if m.Role != dto.ChatRoleAssistant {
			continue
		}
		if content := strings.TrimSpace(m.Content); content != "" {
			out = append(out, content)
		}
	}
	return out
}

func (w *Workflow) citeCandidates(ctx context.Context, companyID int64, req dto.ChatRequest) []string {
	if w.sessions != nil && req.ConversationID > 0 {
		principal, _ := ctxkey.PrincipalFrom(ctx)
		_, values, err := w.sessions.LastCites(ctx, companyID, principal.UserID, req.ConversationID)
		if err != nil {
			logger.FromContext(ctx).Warn("chat: load prior cites", zap.Error(err))
		}
		if len(values) > 0 {
			return values
		}
	}
	return citeCandidatesFromReplies(priorAssistantTurns(req.Messages))
}

func citeCandidatesFromReplies(turns []string) []string {
	for _, pattern := range citeFallbackPatterns {
		if found := tools.CiteCandidatesFromTurns(turns, pattern); len(found) > 0 {
			return found
		}
	}
	return nil
}

var citeFallbackPatterns = []string{`REP-\d+`}

func (w *Workflow) persistTurns(ctx context.Context, companyID int64, req dto.ChatRequest, resp *dto.ChatResponse, tool *skeleton.Response) {
	if w.sessions == nil || req.ConversationID <= 0 {
		return
	}
	principal, _ := ctxkey.PrincipalFrom(ctx)
	log := logger.FromContext(ctx)

	sessionID, err := w.sessions.EnsureSession(ctx, companyID, principal.UserID, req.ConversationID)
	if err != nil {
		log.Warn("chat: ensure session", zap.Error(err))
		return
	}
	resp.ConversationID = sessionID

	if err := w.sessions.AppendTurn(ctx, chatsession.Turn{
		SessionID: sessionID,
		Role:      "user",
	}); err != nil {
		log.Warn("chat: append user turn", zap.Error(err))
		return
	}

	turn := chatsession.Turn{
		SessionID: sessionID,
		Role:      "assistant",

		Outcome: resp.Status,
	}
	if tool != nil {
		turn.CiteKey, turn.CiteValues, turn.ToolID = tool.Cite, tool.CiteValues, tool.ToolID
	}
	if err := w.sessions.AppendTurn(ctx, turn); err != nil {
		log.Warn("chat: append assistant turn", zap.Error(err))
	}
}

func (w *Workflow) shortCircuit(locale string, d intent.Decision, kind decide.Kind, seed uint64) (dto.ChatResponse, bool) {
	switch kind {
	case decide.KindHandoff:
		reply := handoffReply(locale)
		if pooled, ok := w.pool.pick("handoff", "", locale, seed); ok {
			reply = pooled
		}
		return dto.ChatResponse{
			Reply:    reply,
			Status:   dto.ChatStatusHandoff,
			Activity: activity(locale, "request_checked", "permission_checked"),
		}, true
	case decide.KindSocial:
		reply := socialReply(locale)
		if pooled, ok := w.pool.pick("social", d.SubIntent, locale, seed); ok {
			reply = pooled
		}
		return dto.ChatResponse{
			Reply:    reply,
			Status:   dto.ChatStatusAnswered,
			Activity: activity(locale, "request_checked"),
		}, true
	case decide.KindOffTopic:
		reply := offTopicReply(locale)
		if pooled, ok := w.pool.pick("offtopic", "", locale, seed); ok {
			reply = pooled
		}
		return dto.ChatResponse{
			Reply:    reply,
			Status:   dto.ChatStatusAnswered,
			Activity: activity(locale, "request_checked"),
		}, true
	default:
		return dto.ChatResponse{}, false
	}
}

func authoritativeIntent(i intent.Intent) bool {
	switch i {
	case intent.CaseStatus, intent.OpenCase:
		return true
	default:
		return false
	}
}

func toolActor(ctx context.Context, companyID int64) skeleton.Actor {
	principal, _ := ctxkey.PrincipalFrom(ctx)
	return skeleton.Actor{
		CompanyID: companyID,
		UserID:    principal.UserID,
		Perms:     principal.Perms,
		Coverage:  ctxkey.CoverageOrSelf(ctx, companyID),
	}
}
