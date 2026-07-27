package chat

import (
	"context"
	"strings"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/domain/ports"
	apperr "github.com/my/app/internal/shared/errors"
)

// NOTE: drafting not in case
// RunStream mirrors Run's pre-stages
// final LLM answer as plain-text deltas instead of parsing a JSON turn.
func (w *Workflow) RunStream(ctx context.Context, req dto.ChatRequest, emit func(dto.ChatStreamEvent) error) error {
	if err := req.Validate(); err != nil {
		return err
	}

	pre, err := w.prepare(ctx, req, nil)
	if err != nil {
		return err
	}
	if pre.handled {
		return emitFinal(emit, pre.resp)
	}
	companyID, lastUser := pre.companyID, pre.lastUser

	sources, knowledge := w.knowledgeContext(ctx, companyID, lastUser)

	var profileBlock string
	if w.profile != nil {
		if block, perr := w.profile.Build(ctx, companyID); perr == nil {
			profileBlock = block
		}
	}

	prompt, err := w.streamTmpl.Render(map[string]string{
		"language":   promptLanguage(req.Locale),
		"transcript": buildTranscript(req.Messages),
		"knowledge":  knowledgeSection(knowledge),
		"profile":    profileSection(profileBlock),
	})
	if err != nil {
		return emit(dto.ChatStreamEvent{
			Type:    dto.ChatStreamEventError,
			Code:    string(apperr.CodeInternal),
			Message: "chat: render prompt failed",
		})
	}
	prompt.Attachments = toPromptAttachments(lastUserAttachments(req))

	provider, err := w.resolve(ctx, companyID)
	if err != nil {
		return emit(dto.ChatStreamEvent{
			Type:    dto.ChatStreamEventError,
			Code:    string(apperr.CodeUpstreamFailed),
			Message: "chat: unable to resolve a model provider",
		})
	}

	doneEvent := func() dto.ChatStreamEvent {
		return dto.ChatStreamEvent{
			Type:     dto.ChatStreamEventDone,
			Status:   dto.ChatStatusAnswered,
			Sources:  sources,
			Activity: activity(req.Locale, "request_checked", "permission_checked", "searched_knowledge", "answer_prepared"),
		}
	}

	ch, streamErr := provider.Stream(ctx, prompt)
	if streamErr != nil {
		return w.streamFallback(ctx, provider, prompt, emit, doneEvent)
	}

	for chunk := range ch {
		if chunk.Text == "" {
			continue
		}
		if err := emit(dto.ChatStreamEvent{Type: dto.ChatStreamEventDelta, Text: chunk.Text}); err != nil {
			return err
		}
	}

	return emit(doneEvent())
}

// streamFallback LLM generated w/ fallback messages
func (w *Workflow) streamFallback(ctx context.Context, provider ports.LLMProvider, prompt ports.Prompt, emit func(dto.ChatStreamEvent) error, doneEvent func() dto.ChatStreamEvent) error {
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
	return emit(doneEvent())
}

// Turns a pre-stage short-circuit response <Tools>
func emitFinal(emit func(dto.ChatStreamEvent) error, resp dto.ChatResponse) error {
	if strings.TrimSpace(resp.Reply) != "" {
		if err := emit(dto.ChatStreamEvent{Type: dto.ChatStreamEventDelta, Text: resp.Reply}); err != nil {
			return err
		}
	}
	return emit(dto.ChatStreamEvent{
		Type:          dto.ChatStreamEventDone,
		Status:        resp.Status,
		Sources:       resp.Sources,
		Activity:      resp.Activity,
		SearchResults: resp.SearchResults,
	})
}
