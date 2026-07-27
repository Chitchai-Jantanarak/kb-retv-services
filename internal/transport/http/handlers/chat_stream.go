package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/application/dto"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/transport/http/response"
)

type chatStreamWorkflow interface {
	RunStream(ctx context.Context, req dto.ChatRequest, emit func(dto.ChatStreamEvent) error) error
}

type ChatStreamHandler struct {
	workflow chatStreamWorkflow
}

func NewChatStreamHandler(workflow chatStreamWorkflow) *ChatStreamHandler {
	return &ChatStreamHandler{workflow: workflow}
}

func (h *ChatStreamHandler) Create(c *echo.Context) error {
	var req dto.ChatRequest
	if err := c.Bind(&req); err != nil {
		return response.WriteError(c, apperr.Wrap(apperr.CodeInvalidInput, "invalid JSON body", err))
	}
	if err := req.Validate(); err != nil {
		return response.WriteError(c, err)
	}

	w := c.Response()
	w.Header().Set(echo.HeaderContentType, "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, canFlush := w.(http.Flusher)

	emit := func(evt dto.ChatStreamEvent) error {
		payload, err := json.Marshal(evt)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, payload); err != nil {
			return err
		}
		if canFlush {
			flusher.Flush()
		}
		return nil
	}

	if err := h.workflow.RunStream(c.Request().Context(), req, emit); err != nil {
		_ = emit(dto.ChatStreamEvent{
			Type:    dto.ChatStreamEventError,
			Code:    string(apperr.CodeInternal),
			Message: "chat: stream failed",
		})
	}
	return nil
}
