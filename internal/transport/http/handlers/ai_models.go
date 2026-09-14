package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/transport/http/response"
)

type taskModelReader interface {
	TaskModels(context.Context, int64, string) (llm.TaskModels, error)
}

type ChatModelsHandler struct{ reader taskModelReader }

func NewChatModelsHandler(reader taskModelReader) *ChatModelsHandler {
	return &ChatModelsHandler{reader: reader}
}

func (h *ChatModelsHandler) List(c *echo.Context) error {
	ctx := c.Request().Context()
	cid, ok := ctxkey.CompanyID(ctx)
	if !ok || cid <= 0 {
		return response.WriteError(c, apperr.New(apperr.CodeForbidden, "company context required"))
	}
	models, err := h.reader.TaskModels(ctx, cid, "chat")
	if err != nil {
		return response.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, response.OK(models))
}

type AIModelsHandler struct{ catalog *llm.ModelCatalog }

func NewAIModelsHandler(catalog *llm.ModelCatalog) *AIModelsHandler {
	return &AIModelsHandler{catalog: catalog}
}

func (h *AIModelsHandler) List(c *echo.Context) error { return h.handle(c, false) }

func (h *AIModelsHandler) Refresh(c *echo.Context) error { return h.handle(c, true) }

func (h *AIModelsHandler) handle(c *echo.Context, refresh bool) error {
	ctx := c.Request().Context()
	cid, ok := ctxkey.CompanyID(ctx)
	if !ok || cid <= 0 {
		return response.WriteError(c, apperr.New(apperr.CodeForbidden, "company context required"))
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return response.WriteError(c, apperr.New(apperr.CodeInvalidInput, "invalid connection ID"))
	}
	var result llm.CatalogSnapshot
	if refresh {
		result, err = h.catalog.Refresh(ctx, cid, id)
	} else {
		result, err = h.catalog.List(ctx, cid, id)
	}
	if err != nil {
		if _, known := apperr.As(err); !known {
			err = apperr.New(apperr.CodeUnavailable, "model catalog unavailable")
		}
		return response.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, response.OK(result))
}
