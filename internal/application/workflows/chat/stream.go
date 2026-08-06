package chat

import (
	"context"
	"strings"

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
		resp := withChatTimings(pre.resp, timings)
		resp = attachChatDebug(resp, pre.toolDebug, timings, false)
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
		return w.streamFallback(ctx, provider, prompt, emit, doneEvent)
	}

	var model, vendor string
	for chunk := range ch {
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

	return emit(doneEvent(model, vendor))
}

// streamFallback LLM generated w/ fallback messages
func (w *Workflow) streamFallback(ctx context.Context, provider ports.LLMProvider, prompt ports.Prompt, emit func(dto.ChatStreamEvent) error, doneEvent func(string, string) dto.ChatStreamEvent) error {
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
	return emit(doneEvent(completion.Model, completion.Vendor))
}

// Turns a pre-stage short-circuit response <Tools>
func emitFinal(emit func(dto.ChatStreamEvent) error, resp dto.ChatResponse, transcript string) error {
	if strings.TrimSpace(resp.Reply) != "" {
		if err := emit(dto.ChatStreamEvent{Type: dto.ChatStreamEventDelta, Text: resp.Reply}); err != nil {
			return err
		}
	}
	return emit(dto.ChatStreamEvent{
		Type:          dto.ChatStreamEventDone,
		Status:        resp.Status,
		Model:         resp.Model,
		Vendor:        resp.Vendor,
		Sources:       resp.Sources,
		Activity:      resp.Activity,
		SearchResults: resp.SearchResults,
		ToolResult:    resp.ToolResult,
		PendingAction: resp.PendingAction,
		Debug:         resp.Debug,
		Transcript:    transcript,
	})
}
