package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/application/workflows/omnichannel"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/transport/http/response"
)

type manualIntakeAssessor interface {
	EnqueueDraft(ctx context.Context, companyID, conversationID int64) (messageID int64, err error)
}

type IntakeAssessHandler struct {
	assessor manualIntakeAssessor
}

func NewIntakeAssessHandler(assessor manualIntakeAssessor) *IntakeAssessHandler {
	return &IntakeAssessHandler{assessor: assessor}
}

type intakeAssessRequest struct {
	ConversationID int64 `json:"conversation_id"`
}

func (h *IntakeAssessHandler) Create(c *echo.Context) error {
	var req intakeAssessRequest
	if err := c.Bind(&req); err != nil {
		return response.WriteError(c, apperr.Wrap(apperr.CodeInvalidInput, "invalid JSON body", err))
	}
	if req.ConversationID <= 0 {
		return response.WriteError(c, apperr.New(apperr.CodeInvalidInput, "conversation_id is required"))
	}

	companyID := ctxkey.MustCompanyID(c.Request().Context())
	messageID, err := h.assessor.EnqueueDraft(c.Request().Context(), companyID, req.ConversationID)
	if errors.Is(err, omnichannel.ErrDraftNotFound) {
		return response.WriteError(c, apperr.Wrap(apperr.CodeNotFound, "pending draft not found", err))
	}
	if err != nil {
		return response.WriteError(c, err)
	}

	return c.JSON(http.StatusAccepted, response.OK(map[string]any{
		"conversation_id": req.ConversationID,
		"message_id":      messageID,
		"status":          "queued",
	}))
}
