package dto

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	apperr "github.com/my/app/internal/shared/errors"
)

func TestJobEnvelopeValidateRequiresCompanyTypeAndPayload(t *testing.T) {
	payload := json.RawMessage(`{"message":"hello"}`)
	req := JobEnvelope{
		CompanyID: 42,
		Type:      "reply.generate",
		Payload:   payload,
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if req.CompanyID != 42 {
		t.Fatalf("CompanyID = %d, want 42", req.CompanyID)
	}
}

func TestJobEnvelopeValidateRejectsMissingPayload(t *testing.T) {
	req := JobEnvelope{CompanyID: 42, Type: "reply.generate"}

	err := req.Validate()
	if !hasAppCode(err, apperr.CodeInvalidInput) {
		t.Fatalf("Validate() error = %v, want invalid_input", err)
	}
}

func TestActionGenerationRequestValidate(t *testing.T) {
	req := ActionGenerationRequest{
		CompanyID:  42,
		AgentID:    "agent-1",
		MessageID:  "message-1",
		ActionType: "draft_reply",
		Input: map[string]any{
			"intent": "troubleshooting",
		},
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestActionGenerationRequestValidateRejectsUnknownAction(t *testing.T) {
	req := ActionGenerationRequest{
		CompanyID:  42,
		AgentID:    "agent-1",
		MessageID:  "message-1",
		ActionType: "unknown",
		Input:      map[string]any{"intent": "x"},
	}

	err := req.Validate()
	if !hasAppCode(err, apperr.CodeInvalidInput) {
		t.Fatalf("Validate() error = %v, want invalid_input", err)
	}
}

func TestFeedbackRequestValidate(t *testing.T) {
	req := FeedbackRequest{
		CompanyID: 42,
		ActionID:  "action-1",
		Score:     5,
		Comment:   "good",
		CreatedAt: time.Now(),
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestFeedbackRequestValidateRejectsOutOfRangeScore(t *testing.T) {
	req := FeedbackRequest{
		CompanyID: 42,
		ActionID:  "action-1",
		Score:     8,
	}

	err := req.Validate()
	if !hasAppCode(err, apperr.CodeInvalidInput) {
		t.Fatalf("Validate() error = %v, want invalid_input", err)
	}
}

func hasAppCode(err error, code apperr.Code) bool {
	var appErr *apperr.AppError
	return errors.As(err, &appErr) && appErr.Code == code
}
