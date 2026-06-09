package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/application/workflows/promote"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/transport/http/response"
)

type reviewRejector interface {
	MarkRejected(ctx context.Context, companyID, id int64, reason string) error
}

type ReviewHandler struct {
	promoter *promote.Workflow
	rejector reviewRejector
}

func NewReviewHandler(promoter *promote.Workflow, rejector reviewRejector) *ReviewHandler {
	return &ReviewHandler{promoter: promoter, rejector: rejector}
}

func (h *ReviewHandler) Approve(c *echo.Context) error {
	cid := ctxkey.MustCompanyID(c.Request().Context())
	id, err := parseIDParam(c, "id")
	if err != nil {
		return response.WriteError(c, err)
	}
	res, err := h.promoter.RunFor(c.Request().Context(), cid, id)
	if err != nil {
		return response.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, response.OK(map[string]any{
		"review_item_id": res.ReviewItemID,
		"kind":           res.Kind,
		"promotion_ref":  res.PromotionRef,
	}))
}

type rejectRequest struct {
	Reason string `json:"reason,omitempty"`
}

func (h *ReviewHandler) Reject(c *echo.Context) error {
	cid := ctxkey.MustCompanyID(c.Request().Context())
	id, err := parseIDParam(c, "id")
	if err != nil {
		return response.WriteError(c, err)
	}
	var req rejectRequest
	_ = c.Bind(&req)
	if err := h.rejector.MarkRejected(c.Request().Context(), cid, id, req.Reason); err != nil {
		return response.WriteError(c, err)
	}
	return c.JSON(http.StatusOK, response.OK(map[string]any{
		"review_item_id": id,
		"status":         "rejected",
	}))
}

func parseIDParam(c *echo.Context, name string) (int64, error) {
	raw := c.Param(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.New(apperr.CodeInvalidInput, "invalid id path param")
	}
	return id, nil
}
