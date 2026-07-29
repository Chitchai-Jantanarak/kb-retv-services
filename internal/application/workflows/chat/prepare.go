package chat

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/intent"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/shared/logger"
)

type chatPreamble struct {
	companyID  int64
	lastUser   string
	resp       dto.ChatResponse
	handled    bool
	transcript string
}

// prepare runs the pre-stages shared by Run and RunStream: request scope
// checks, audio transcription, the fast off-domain guard, intent routing,
// and the tool orchestrator short-circuit. It returns handled=true when the
// caller should use resp as the final answer without touching the LLM.
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

	fastGuardHit := false
	timedChat(timings, "fast_guard", func() {
		fastGuardHit = fastGuardOffDomain(lastUser)
	})
	if fastGuardHit {
		return chatPreamble{
			companyID:  companyID,
			lastUser:   lastUser,
			handled:    true,
			transcript: transcript,
			resp: dto.ChatResponse{
				Reply:    offDomainReply(req.Locale),
				Status:   dto.ChatStatusOffDomain,
				Activity: activity(req.Locale, "request_checked"),
			},
		}, nil
	}

	var decision intent.Decision
	err = timedChatErr(timings, "router", func() error {
		var err error
		decision, err = w.router.Route(ctx, lastUser)
		return err
	})
	if err != nil {
		return chatPreamble{}, err
	}
	if w.orch != nil && decision.Intent != intent.OffDomain && decision.Intent != intent.Handoff {
		var toolResp skeleton.Response
		var toolErr error
		timedChat(timings, "tool", func() {
			toolResp, toolErr = w.orch.Handle(ctx, toolActor(ctx, companyID), lastUser)
		})
		if toolErr != nil {
			logger.FromContext(ctx).Error("chat: tool orchestrator failed", zap.Int64("company_id", companyID), zap.Error(toolErr))
		}
		if toolErr != nil && toolResp.Kind == "write" {
			return chatPreamble{
				companyID:  companyID,
				lastUser:   lastUser,
				handled:    true,
				transcript: transcript,
				resp: dto.ChatResponse{
					Reply:    toolFailedReply(req.Locale),
					Status:   dto.ChatStatusToolFailed,
					Activity: activity(req.Locale, "request_checked", "permission_checked"),
				},
			}, nil
		}
		if toolErr == nil && toolResp.Matched {
			return chatPreamble{
				companyID:  companyID,
				lastUser:   lastUser,
				handled:    true,
				transcript: transcript,
				resp:       toolChatResponse(req.Locale, toolResp),
			}, nil
		}
		if authoritativeIntent(decision.Intent) {
			return chatPreamble{
				companyID:  companyID,
				lastUser:   lastUser,
				handled:    true,
				transcript: transcript,
				resp: dto.ChatResponse{
					Reply:    handoffReply(req.Locale),
					Status:   dto.ChatStatusHandoff,
					Activity: activity(req.Locale, "request_checked", "permission_checked"),
				},
			}, nil
		}
	}
	if resp, handled := w.shortCircuit(req.Locale, decision); handled {
		return chatPreamble{companyID: companyID, lastUser: lastUser, handled: true, transcript: transcript, resp: resp}, nil
	}

	return chatPreamble{companyID: companyID, lastUser: lastUser, transcript: transcript}, nil
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

func (w *Workflow) shortCircuit(locale string, d intent.Decision) (dto.ChatResponse, bool) {
	switch d.Intent {
	case intent.OffDomain:
		return dto.ChatResponse{
			Reply:    offDomainReply(locale),
			Status:   dto.ChatStatusOffDomain,
			Activity: activity(locale, "request_checked"),
		}, true
	case intent.Handoff:
		return dto.ChatResponse{
			Reply:    handoffReply(locale),
			Status:   dto.ChatStatusHandoff,
			Activity: activity(locale, "request_checked", "permission_checked"),
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
