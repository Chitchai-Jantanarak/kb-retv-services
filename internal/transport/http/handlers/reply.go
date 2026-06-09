package handlers

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/application/dto"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/transport/http/response"
)

type replyWorkflow interface {
	Run(ctx context.Context, req dto.ReplyRequest) (dto.ReplyResponse, error)
}

type ReplyHandler struct {
	workflow replyWorkflow
}

func NewReplyHandler(workflow replyWorkflow) *ReplyHandler {
	return &ReplyHandler{workflow: workflow}
}

func (h *ReplyHandler) Create(c *echo.Context) error {
	var req dto.ReplyRequest
	if err := c.Bind(&req); err != nil {
		return response.WriteError(c, apperr.Wrap(apperr.CodeInvalidInput, "invalid JSON body", err))
	}

	if err := req.Validate(); err != nil {
		return response.WriteError(c, err)
	}

	reply, err := h.workflow.Run(c.Request().Context(), req)
	if err != nil {
		return response.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, response.OK(reply))
}
