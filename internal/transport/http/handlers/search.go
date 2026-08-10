package handlers

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/application/dto"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/transport/http/response"
)

type searchWorkflow interface {
	Run(ctx context.Context, req dto.SearchRequest) (dto.SearchResponse, error)
}

type SearchHandler struct {
	workflow searchWorkflow
}

func NewSearchHandler(workflow searchWorkflow) *SearchHandler {
	return &SearchHandler{workflow: workflow}
}

func (h *SearchHandler) Create(c *echo.Context) error {
	var req dto.SearchRequest
	if err := c.Bind(&req); err != nil {
		return response.WriteError(c, apperr.Wrap(apperr.CodeInvalidInput, "invalid JSON body", err))
	}

	if err := req.Validate(); err != nil {
		return response.WriteError(c, err)
	}

	resp, err := h.workflow.Run(c.Request().Context(), req)
	if err != nil {
		return response.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, response.OK(resp))
}
