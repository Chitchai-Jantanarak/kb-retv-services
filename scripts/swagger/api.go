package main

import (
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/transport/http/response"
)

var (
	_ = dto.ReplyRequest{}
	_ = response.Envelope{}
)

type lineInboundWebhookRequest struct {
	Destination string             `json:"destination" example:"U1234567890abcdef1234567890abcdef"`
	Events      []lineInboundEvent `json:"events"`
}

type lineInboundEvent struct {
	Type       string             `json:"type" example:"message"`
	Timestamp  int64              `json:"timestamp" example:"1716172800000"`
	Source     lineInboundSource  `json:"source"`
	Message    lineInboundMessage `json:"message"`
	ReplyToken string             `json:"replyToken" example:"reply-token-example"`
}

type lineInboundSource struct {
	Type    string `json:"type" example:"user"`
	UserID  string `json:"userId,omitempty" example:"U4af4980629"`
	GroupID string `json:"groupId,omitempty" example:"C4af4980629"`
	RoomID  string `json:"roomId,omitempty" example:"R4af4980629"`
}

type lineInboundMessage struct {
	ID   string `json:"id" example:"519551372899"`
	Type string `json:"type" example:"text"`
	Text string `json:"text" example:"My internet is offline"`
}

type emailInboundWebhookRequest struct {
	MessageID string `json:"message_id" example:"email-20260520-001"`
	From      string `json:"from" example:"Customer One <customer@example.com>"`
	To        string `json:"to" example:"support@example.com"`
	Subject   string `json:"subject" example:"Internet connection issue"`
	Body      string `json:"body" example:"The connection has been unstable since this morning."`
	BodyHTML  string `json:"body_html,omitempty" example:"<p>The connection has been unstable since this morning.</p>"`
}

type feedbackRequest struct {
	AIActionID int64  `json:"ai_action_id" example:"42"`
	Verdict    string `json:"verdict" example:"accepted" enums:"accepted,edited,rejected,escalated"`
	Note       string `json:"note,omitempty" example:"Agent sent the draft unchanged."`
}

type reviewRejectRequest struct {
	Reason string `json:"reason,omitempty" example:"The generated article duplicated an existing answer."`
}

type reviewQueueCallbackRequest struct {
	CompanyID   int64                  `json:"company_id" example:"1"`
	Kind        string                 `json:"kind" example:"symptom_proposed" enums:"kb_promotion,symptom_proposed,subject_proposed,kb_gap"`
	PayloadHash string                 `json:"payload_hash,omitempty" example:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`
	Payload     map[string]interface{} `json:"payload"`
	SourceRefs  map[string]interface{} `json:"source_refs,omitempty"`
}

type reviewQueueCallbackResponse struct {
	Data reviewQueueCallbackData `json:"data"`
}

type reviewQueueCallbackData struct {
	ID        int64  `json:"id" example:"123"`
	CompanyID int64  `json:"company_id" example:"1"`
	Kind      string `json:"kind" example:"symptom_proposed"`
}

// @title Centric RAG AI Service
// @version 0.1.0
// @description Internal AI service for tenancy-scoped support retrieval and reply workflows.
// @BasePath /
func main() {}

// @Summary Health check
// @Tags system
// @Produce json
// @Success 200 {object} map[string]string
// @Router /healthz [get]
func healthz() {}

// @Summary Create reply
// @Tags reply
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param request body dto.ReplyRequest true "Reply request"
// @Success 200 {object} response.Envelope
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/reply [post]
func createReply() {}

// @Summary Receive LINE inbound webhook
// @Tags inbound
// @Accept json
// @Produce json
// @Param request body lineInboundWebhookRequest true "LINE webhook payload"
// @Success 200 {object} response.Envelope
// @Failure 400 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/inbound/line [post]
func receiveLineInbound() {}

// @Summary Receive email inbound webhook
// @Tags inbound
// @Accept json
// @Produce json
// @Param request body emailInboundWebhookRequest true "Email webhook payload"
// @Success 200 {object} response.Envelope
// @Failure 400 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/inbound/email [post]
func receiveEmailInbound() {}

// @Summary Record reply feedback
// @Tags reply
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param request body feedbackRequest true "Smart-reply feedback"
// @Success 200 {object} response.Envelope
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/reply/feedback [post]
func createFeedback() {}

// @Summary Reject review queue item
// @Tags review-queue
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param id path int true "Review item ID"
// @Param request body reviewRejectRequest true "Review rejection"
// @Success 200 {object} response.Envelope
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/admin/review-queue/{id}/reject [post]
func rejectReviewQueueItem() {}

// @Summary Receive signed Laravel review queue callback
// @Description Laravel internal endpoint used by the Go review outbox. `X-AI-Signature` is lowercase hex HMAC-SHA256 over `<timestamp>.<raw JSON body>` using `GO_AI_WEBHOOK_SECRET`.
// @Tags laravel-callbacks
// @Accept json
// @Produce json
// @Param X-AI-Timestamp header string true "Unix seconds timestamp, accepted within GO_AI_WEBHOOK_TOLERANCE"
// @Param X-AI-Signature header string true "Lowercase hex HMAC-SHA256 of '<timestamp>.<raw body>'"
// @Param request body reviewQueueCallbackRequest true "Review queue callback"
// @Success 200 {object} reviewQueueCallbackResponse "Existing idempotent review item"
// @Success 201 {object} reviewQueueCallbackResponse "Created review queue item"
// @Failure 401 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 422 {object} response.Envelope
// @Failure 503 {object} response.Envelope
// @Router /internal/review-queue [post]
func receiveReviewQueueCallback() {}
