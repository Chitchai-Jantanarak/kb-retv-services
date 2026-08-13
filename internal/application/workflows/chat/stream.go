package chat

import (
	"context"
	"strings"
	"time"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
)

// NOTE: drafting not in case
// RunStream mirrors Run's pre-stages
// final LLM answer as plain-text deltas instead of parsing a JSON turn.
func (w *Workflow) RunStream(ctx context.Context, req dto.ChatRequest, emit func(dto.ChatStreamEvent) error) error {
	if err := req.Validate(); err != nil {
		return err
	}
	principal, _ := ctxkey.PrincipalFrom(ctx)
	req.Debug = authorizeChatDebug(principal, req.Debug)
	req.Messages = append([]dto.ChatMessage(nil), req.Messages...)
	timings := chatTimings(req.Debug)
	started := time.Now()

	pre, err := w.prepare(ctx, req, timings)
	if err != nil {
		return err
	}
	if pre.transcript != "" {
		if err := emit(dto.ChatStreamEvent{Type: dto.ChatStreamEventTranscript, Text: pre.transcript}); err != nil {
			return err
		}
	}
	if req.Debug && pre.toolDebug != nil {
		if err := emit(dto.ChatStreamEvent{
			Type:  dto.ChatStreamEventDebug,
			Debug: attachChatDebug(dto.ChatResponse{}, pre.toolDebug, timings, false).Debug,
		}); err != nil {
			return err
		}
	}
	if pre.handled {
		w.finalizeTurn(ctx, pre.resp.Status, pre, started, 0, 0, detectLang(pre.lastUser))
		resp := withChatTimings(pre.resp, timings)
		resp = attachChatDebug(resp, pre.toolDebug, timings, false)
		w.persistTurns(ctx, pre.companyID, req, &resp, pre.tool)
		return emitFinal(emit, resp, pre.transcript)
	}
	companyID, lastUser := pre.companyID, pre.lastUser

	sources, knowledge, profileBlock := w.fetchContext(ctx, companyID, lastUser, timings)

	var prompt ports.Prompt
	err = timedChatErr(timings, "render", func() error {
		var renderErr error
		prompt, renderErr = w.streamTmpl.Render(map[string]string{
			"language":   promptLanguage(req.Locale),
			"transcript": buildTranscript(req.Messages),
			"knowledge":  knowledgeSection(knowledge),
			"profile":    profileSection(profileBlock),
		})
		return renderErr
	})
	if err != nil {
		return emit(dto.ChatStreamEvent{
			Type:    dto.ChatStreamEventError,
			Code:    string(apperr.CodeInternal),
			Message: "chat: render prompt failed",
		})
	}
	prompt.Attachments = toPromptAttachments(lastUserAttachments(req))

	var provider ports.LLMProvider
	err = timedChatErr(timings, "resolve", func() error {
		var resolveErr error
		provider, resolveErr = w.resolve(ctx, companyID)
		return resolveErr
	})
	if err != nil {
		return emit(dto.ChatStreamEvent{
			Type:    dto.ChatStreamEventError,
			Code:    string(apperr.CodeUpstreamFailed),
			Message: "chat: unable to resolve a model provider",
		})
	}

	doneEvent := func(model string, vendor string) dto.ChatStreamEvent {
		return dto.ChatStreamEvent{
			Type:       dto.ChatStreamEventDone,
			Status:     dto.ChatStatusAnswered,
			Model:      model,
			Vendor:     vendor,
			Sources:    sources,
			Activity:   activity(req.Locale, "request_checked", "permission_checked", "searched_knowledge", "answer_prepared"),
			Debug:      attachChatDebug(dto.ChatResponse{}, pre.toolDebug, timings, false).Debug,
			Transcript: pre.transcript,
		}
	}

	var ch <-chan ports.Completion
	streamErr := timedChatErr(timings, "generate", func() error {
		var generateErr error
		ch, generateErr = provider.Stream(ctx, prompt)
		return generateErr
	})
	if streamErr != nil {
		return w.streamFallback(ctx, provider, prompt, req, emit, doneEvent, pre, started)
	}

	var model, vendor string
	var tokensIn, tokensOut int
	for chunk := range ch {
		if chunk.Usage.Input > 0 {
			tokensIn = chunk.Usage.Input
		}
		if chunk.Usage.Output > 0 {
			tokensOut = chunk.Usage.Output
		}
		if chunk.Text == "" {
			continue
		}
		if chunk.Model != "" {
			model = chunk.Model
		}
		if chunk.Vendor != "" {
			vendor = chunk.Vendor
		}
		if err := emit(dto.ChatStreamEvent{Type: dto.ChatStreamEventDelta, Text: chunk.Text}); err != nil {
			return err
		}
	}

	w.finalizeTurn(ctx, dto.ChatStatusAnswered, pre, started, tokensIn, tokensOut, detectLang(pre.lastUser))
	final := dto.ChatResponse{Status: dto.ChatStatusAnswered}
	w.persistTurns(ctx, companyID, req, &final, pre.tool)
	ev := doneEvent(model, vendor)
	ev.ConversationID = final.ConversationID
	return emit(ev)
}

// streamFallback LLM generated w/ fallback messages
func (w *Workflow) streamFallback(ctx context.Context, provider ports.LLMProvider, prompt ports.Prompt, req dto.ChatRequest, emit func(dto.ChatStreamEvent) error, doneEvent func(string, string) dto.ChatStreamEvent, pre chatPreamble, started time.Time) error {
	completion, err := provider.Generate(ctx, prompt)
	if err != nil || strings.TrimSpace(completion.Text) == "" {
		return emit(dto.ChatStreamEvent{
			Type:    dto.ChatStreamEventError,
			Code:    string(apperr.CodeUpstreamFailed),
			Message: "chat: unable to generate a response",
		})
	}
	if err := emit(dto.ChatStreamEvent{Type: dto.ChatStreamEventDelta, Text: completion.Text}); err != nil {
		return err
	}
	w.finalizeTurn(ctx, dto.ChatStatusAnswered, pre, started, completion.Usage.Input, completion.Usage.Output, detectLang(pre.lastUser))
	final := dto.ChatResponse{Status: dto.ChatStatusAnswered}
	w.persistTurns(ctx, pre.companyID, req, &final, pre.tool)
	ev := doneEvent(completion.Model, completion.Vendor)
	ev.ConversationID = final.ConversationID
	return emit(ev)
}

// Turns a pre-stage short-circuit response <Tools>
func emitFinal(emit func(dto.ChatStreamEvent) error, resp dto.ChatResponse, transcript string) error {
	if strings.TrimSpace(resp.Reply) != "" {
		if err := emit(dto.ChatStreamEvent{Type: dto.ChatStreamEventDelta, Text: resp.Reply}); err != nil {
			return err
		}
	}
	return emit(dto.ChatStreamEvent{
		Type:           dto.ChatStreamEventDone,
		Status:         resp.Status,
		Model:          resp.Model,
		Vendor:         resp.Vendor,
		Sources:        resp.Sources,
		Activity:       resp.Activity,
		SearchResults:  resp.SearchResults,
		ToolResult:     resp.ToolResult,
		PendingAction:  resp.PendingAction,
		Debug:          resp.Debug,
		Transcript:     transcript,
		ConversationID: resp.ConversationID,
	})
}
