package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/transport/http/response"
)

type feedbackRecorder interface {
	RecordFeedback(ctx context.Context, fb ports.AIActionFeedback) error
}

type FeedbackHandler struct {
	recorder feedbackRecorder
}

func NewFeedbackHandler(recorder feedbackRecorder) *FeedbackHandler {
	return &FeedbackHandler{recorder: recorder}
}

type feedbackRequest struct {
	AIActionID int64  `json:"ai_action_id"`
	Verdict    string `json:"verdict"`
	Note       string `json:"note,omitempty"`
}

func (h *FeedbackHandler) Create(c *echo.Context) error {
	var req feedbackRequest
	if err := c.Bind(&req); err != nil {
		return response.WriteError(c, apperr.Wrap(apperr.CodeInvalidInput, "invalid JSON body", err))
	}
	if req.AIActionID <= 0 {
		return response.WriteError(c, apperr.New(apperr.CodeInvalidInput, "ai_action_id is required"))
	}
	score, ok := scoreFor(req.Verdict)
	if !ok {
		return response.WriteError(c, apperr.New(apperr.CodeInvalidInput, "verdict must be accepted|edited|rejected|escalated"))
	}

	cid := ctxkey.MustCompanyID(c.Request().Context())
	fb := ports.AIActionFeedback{
		CompanyID: cid,
		ActionID:  req.AIActionID,
		Verdict:   req.Verdict,
		Score:     score,
		Note:      strings.TrimSpace(req.Note),
	}
	if err := h.recorder.RecordFeedback(c.Request().Context(), fb); err != nil {
		return response.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, response.OK(map[string]any{
		"ai_action_id": req.AIActionID,
		"verdict":      req.Verdict,
		"score":        score,
	}))
}

func scoreFor(verdict string) (int8, bool) {
	switch strings.ToLower(verdict) {
	case "accepted":
		return 2, true
	case "edited":
		return 1, true
	case "escalated":
		return 0, true
	case "rejected":
		return -1, true
	}
	return 0, false
}
