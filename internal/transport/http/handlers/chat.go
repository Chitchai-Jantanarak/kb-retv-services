package handlers

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/application/dto"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/transport/http/response"
)

type chatWorkflow interface {
	Run(ctx context.Context, req dto.ChatRequest) (dto.ChatResponse, error)
}

type ChatHandler struct {
	workflow chatWorkflow
}

func NewChatHandler(workflow chatWorkflow) *ChatHandler {
	return &ChatHandler{workflow: workflow}
}

func (h *ChatHandler) Create(c *echo.Context) error {
	var req dto.ChatRequest
	if err := c.Bind(&req); err != nil {
		return response.WriteError(c, apperr.Wrap(apperr.CodeInvalidInput, "invalid JSON body", err))
	}

	resp, err := h.workflow.Run(c.Request().Context(), req)
	if err != nil {
		return response.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, response.OK(resp))
}
