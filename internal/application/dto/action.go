package dto

import (
	"encoding/json"
	"time"

	apperr "github.com/my/app/internal/shared/errors"
)

type JobEnvelope struct {
	CompanyID int64             `json:"company_id" validate:"required,gt=0"`
	TraceID   string            `json:"trace_id,omitempty"`
	Type      string            `json:"type" validate:"required"`
	Payload   json.RawMessage   `json:"payload" validate:"required"`
	Attempt   int               `json:"attempt,omitempty" validate:"omitempty,gte=0"`
	Deadline  time.Time         `json:"deadline,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

func (r *JobEnvelope) Validate() error {
	r.Normalize()
	if len(r.Payload) == 0 {
		return apperr.New(apperr.CodeInvalidInput, "payload is required")
	}
	return validateStruct(r)
}

func (r *JobEnvelope) Normalize() {
	r.TraceID = normalizeText(r.TraceID)
	r.Type = normalizeText(r.Type)
}

type ActionGenerationRequest struct {
	CompanyID  int64          `json:"company_id" validate:"required,gt=0"`
	AgentID    string         `json:"agent_id" validate:"required"`
	MessageID  string         `json:"message_id" validate:"required"`
	ActionType string         `json:"action_type" validate:"required,oneof=draft_reply open_case search escalate summarize"`
	Input      map[string]any `json:"input" validate:"required"`
}

func (r *ActionGenerationRequest) Validate() error {
	r.Normalize()
	return validateStruct(r)
}

func (r *ActionGenerationRequest) Normalize() {
	r.AgentID = normalizeText(r.AgentID)
	r.MessageID = normalizeText(r.MessageID)
	r.ActionType = normalizeText(r.ActionType)
}

type FeedbackRequest struct {
	CompanyID int64     `json:"company_id" validate:"required,gt=0"`
	ActionID  string    `json:"action_id" validate:"required"`
	Score     int       `json:"score" validate:"gte=1,lte=5"`
	Comment   string    `json:"comment,omitempty" validate:"omitempty,max=2000"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

func (r *FeedbackRequest) Validate() error {
	r.Normalize()
	return validateStruct(r)
}

func (r *FeedbackRequest) Normalize() {
	r.ActionID = normalizeText(r.ActionID)
	r.Comment = normalizeText(r.Comment)
}
