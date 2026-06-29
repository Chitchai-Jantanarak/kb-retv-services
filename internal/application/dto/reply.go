package dto

import (
	apperr "github.com/my/app/internal/shared/errors"
)

type ReplyRequest struct {
	CustomerID     string            `json:"customer_id,omitempty" example:"cust_10293"`
	SiteID         string            `json:"site_id,omitempty" example:"site_42"`
	Message        string            `json:"message,omitempty" example:"My internet has been offline since this morning."`
	Query          string            `json:"query,omitempty"`
	Channel        string            `json:"channel,omitempty" validate:"omitempty,oneof=line email web voice api in_app" example:"web" enums:"line,email,web,voice,api,in_app"`
	ConversationID string            `json:"conversation_id,omitempty" example:"conv_8f21"`
	MessageID      string            `json:"message_id,omitempty" example:"msg_1001"`
	Attachments    []AttachmentRef   `json:"attachments,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Mode           string            `json:"mode,omitempty" validate:"omitempty,oneof=fast_draft full_review debug" example:"full_review" enums:"fast_draft,full_review,debug"`
	Debug          bool              `json:"debug,omitempty" example:"false"`
}

func (r *ReplyRequest) Validate() error {
	r.Normalize()
	if err := validateAttachmentRefs(r.Attachments); err != nil {
		return err
	}
	if r.Message == "" {
		return apperr.New(apperr.CodeInvalidInput, "message is required")
	}
	return validateStruct(r)
}

func (r *ReplyRequest) Normalize() {
	r.CustomerID = normalizeText(r.CustomerID)
	r.SiteID = normalizeText(r.SiteID)
	r.Message = normalizeText(r.Message)
	r.Query = normalizeText(r.Query)
	if r.Message == "" {
		r.Message = r.Query
	}
	r.Channel = normalizeText(r.Channel)
	r.ConversationID = normalizeText(r.ConversationID)
	r.MessageID = normalizeText(r.MessageID)
	r.Mode = normalizeText(r.Mode)
}

type ReplyResponse struct {
	Intent         string           `json:"intent"`
	Draft          string           `json:"draft"`
	Suggestion     string           `json:"suggestion"`
	Sources        []ReplySource    `json:"sources"`
	Confidence     float64          `json:"confidence"`
	Decision       string           `json:"decision"`
	Reason         string           `json:"reason,omitempty"`
	RetryAfterMs   int64            `json:"retry_after_ms,omitempty"`
	AIActionID     int64            `json:"ai_action_id,omitempty"`
	StageTimingsMS map[string]int64 `json:"stage_timings_ms,omitempty"`
	DebugTrace     any              `json:"debug_trace,omitempty"`
}

type ReplySource struct {
	ID    string  `json:"id"`
	Title string  `json:"title,omitempty"`
	Score float64 `json:"score,omitempty"`
}
