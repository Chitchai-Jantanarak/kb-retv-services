package handlers

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/application/workflows/instructions"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/transport/http/response"
)

type instructionsEnhancer interface {
	Enhance(ctx context.Context, companyID int64, req instructions.Request) (instructions.Result, error)
}

type InstructionsEnhanceHandler struct {
	enhancer instructionsEnhancer
}

func NewInstructionsEnhanceHandler(enhancer instructionsEnhancer) *InstructionsEnhanceHandler {
	return &InstructionsEnhanceHandler{enhancer: enhancer}
}

type instructionsEnhanceAnswer struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type instructionsEnhanceRequest struct {
	Text    string                      `json:"text"`
	Locale  string                      `json:"locale"`
	Answers []instructionsEnhanceAnswer `json:"answers"`
}

type instructionsEnhanceChange struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
	Why  string `json:"why"`
}

type instructionsEnhanceData struct {
	Enhanced  string                      `json:"enhanced"`
	Changes   []instructionsEnhanceChange `json:"changes"`
	Questions []string                    `json:"questions"`
}

func (h *InstructionsEnhanceHandler) Create(c *echo.Context) error {
	var req instructionsEnhanceRequest
	if err := c.Bind(&req); err != nil {
		return response.WriteError(c, apperr.Wrap(apperr.CodeInvalidInput, "invalid JSON body", err))
	}

	answers := make([]instructions.Answer, 0, len(req.Answers))
	for _, a := range req.Answers {
		if a.Answer == "" {
			continue
		}
		answers = append(answers, instructions.Answer{Question: a.Question, Answer: a.Answer})
	}

	companyID := ctxkey.MustCompanyID(c.Request().Context())
	result, err := h.enhancer.Enhance(c.Request().Context(), companyID, instructions.Request{
		Text:    req.Text,
		Locale:  req.Locale,
		Answers: answers,
	})
	if err != nil {
		return response.WriteError(c, err)
	}

	changes := make([]instructionsEnhanceChange, 0, len(result.Changes))
	for _, ch := range result.Changes {
		changes = append(changes, instructionsEnhanceChange{Kind: ch.Kind, Text: ch.Text, Why: ch.Why})
	}
	questions := result.Questions
	if questions == nil {
		questions = []string{}
	}

	return c.JSON(http.StatusOK, response.OK(instructionsEnhanceData{
		Enhanced:  result.Enhanced,
		Changes:   changes,
		Questions: questions,
	}))
}
