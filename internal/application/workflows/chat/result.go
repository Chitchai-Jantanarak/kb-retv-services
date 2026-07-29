package chat

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
	"github.com/my/app/internal/shared/logger"
)

func buildChatResponse(req dto.ChatRequest, transcript string, turn llmTurn, sources []dto.ChatSource) dto.ChatResponse {
	return dto.ChatResponse{
		Reply:      turn.Reply,
		Status:     dto.ChatStatusAnswered,
		Sources:    sources,
		Activity:   activity(req.Locale, "request_checked", "permission_checked", "searched_knowledge", "answer_prepared"),
		Case:       normalizeCaseDraft(turn.Case),
		Transcript: transcript,
	}
}

func (w *Workflow) applyCaseSearch(
	ctx context.Context,
	locale string,
	companyID int64,
	searchRequest *dto.ChatSearchRequest,
	resp *dto.ChatResponse,
	timings map[string]int64,
) {
	search := normalizeSearch(searchRequest)
	if search == nil || w.cases == nil {
		return
	}

	timedChat(timings, "search", func() {
		coverage := ctxkey.CoverageOrSelf(ctx, companyID)
		results, err := w.cases.SearchCases(ctx, coverage, search.Query, search.Product, search.Status, search.Limit)
		log := logger.FromContext(ctx).With(
			zap.Int64("company_id", companyID),
			zap.String("search_query", search.Query),
			zap.String("search_product", search.Product),
			zap.String("search_status", search.Status),
			zap.Int("search_limit", search.Limit),
		)
		if err != nil {
			log.Error("chat: case search failed", zap.Error(err))
			resp.Status = dto.ChatStatusToolFailed
			resp.Reply = toolFailedReply(locale)
			resp.Sources = nil
			return
		}
		log.Info("chat: case search", zap.Int("rows", len(results)))
		if len(results) == 0 {
			resp.Status = dto.ChatStatusNoResults
			resp.Reply = noResultsReply(locale)
			resp.Sources = nil
			return
		}
		resp.SearchResults = results
	})
}

func toolChatResponse(locale string, r skeleton.Response) dto.ChatResponse {
	reply := r.Headline
	if len(r.Lines) > 0 {
		reply = r.Headline + "\n" + strings.Join(r.Lines, "\n")
	}
	return dto.ChatResponse{
		Reply:    reply,
		Status:   dto.ChatStatusAnswered,
		Activity: activity(locale, "request_checked", "permission_checked", "searched_knowledge", "answer_prepared"),
	}
}

func lastUserAttachments(req dto.ChatRequest) []dto.AttachmentRef {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == dto.ChatRoleUser {
			return req.Messages[i].Attachments
		}
	}
	return nil
}

func toPromptAttachments(attachments []dto.AttachmentRef) []ports.Attachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]ports.Attachment, len(attachments))
	for i, attachment := range attachments {
		out[i] = toPromptAttachment(attachment)
	}
	return out
}

func toPromptAttachment(attachment dto.AttachmentRef) ports.Attachment {
	return ports.Attachment{
		ID:         attachment.ID,
		MIMEType:   attachment.MIMEType,
		StorageKey: attachment.StorageKey,
		URL:        attachment.URL,
		SizeBytes:  attachment.SizeBytes,
	}
}
