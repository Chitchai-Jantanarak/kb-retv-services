package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/transport/http/response"
)

type actionConfirmer interface {
	Confirm(ctx context.Context, actor skeleton.Actor, id string) (skeleton.Response, error)
}

type ChatConfirmHandler struct {
	orch actionConfirmer
}

func NewChatConfirmHandler(orch actionConfirmer) *ChatConfirmHandler {
	return &ChatConfirmHandler{orch: orch}
}

func (h *ChatConfirmHandler) Create(c *echo.Context) error {
	var req dto.ChatConfirmRequest
	if err := c.Bind(&req); err != nil {
		return response.WriteError(c, apperr.Wrap(apperr.CodeInvalidInput, "invalid JSON body", err))
	}
	actionID := strings.TrimSpace(req.ActionID)
	if len(actionID) != 32 {
		return response.WriteError(c, apperr.New(apperr.CodeInvalidInput, "action_id must be a 32-character id"))
	}

	ctx := c.Request().Context()
	companyID, ok := ctxkey.CompanyID(ctx)
	if !ok || companyID <= 0 {
		return response.WriteError(c, apperr.New(apperr.CodeUnauthorized, "company scope is required"))
	}
	principal, _ := ctxkey.PrincipalFrom(ctx)
	actor := skeleton.Actor{
		CompanyID: companyID,
		UserID:    principal.UserID,
		Perms:     principal.Perms,
		Coverage:  ctxkey.CoverageOrSelf(ctx, companyID),
	}

	result, err := h.orch.Confirm(ctx, actor, actionID)
	if err != nil {
		return response.WriteError(c, apperr.Wrap(apperr.CodeInvalidInput, "action not found, expired, or not permitted", err))
	}

	reply := result.Headline
	if len(result.Lines) > 0 {
		reply = result.Headline + "\n" + strings.Join(result.Lines, "\n")
	}
	return c.JSON(http.StatusOK, response.OK(dto.ChatResponse{
		Reply:  reply,
		Status: dto.ChatStatusAnswered,
	}))
}
