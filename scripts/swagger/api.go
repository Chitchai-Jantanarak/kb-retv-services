package main

import (
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/transport/http/response"
)

var (
	_ = dto.ChatRequest{}
	_ = dto.ChatResponse{}
	_ = dto.ReplyRequest{}
	_ = dto.ChatConfirmRequest{}
	_ = dto.ChatStreamEvent{}
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

type reviewApproveData struct {
	ReviewItemID int64  `json:"review_item_id" example:"123"`
	Kind         string `json:"kind" example:"symptom_proposed"`
	PromotionRef string `json:"promotion_ref,omitempty" example:"symptom_node:456"`
}

type reviewApproveResponse struct {
	Data reviewApproveData `json:"data"`
}

type aiAccuracyData struct {
	TotalClassified int64 `json:"total_classified" example:"120"`
	HumanConfirmed  int64 `json:"human_confirmed" example:"98"`
	ProblemTypeHits int64 `json:"problem_type_hits" example:"90"`
	SubjectHits     int64 `json:"subject_hits" example:"95"`
	SymptomHits     int64 `json:"symptom_hits" example:"80"`
	SeverityHits    int64 `json:"severity_hits" example:"88"`
}

type aiAccuracyResponse struct {
	Data aiAccuracyData `json:"data"`
}

type answerRateData struct {
	TotalActions   int64   `json:"total_actions" example:"200"`
	HighConfidence int64   `json:"high_confidence" example:"170"`
	LatencyAvgMS   int64   `json:"latency_avg_ms" example:"850"`
	Threshold      float64 `json:"threshold" example:"0.85"`
}

type answerRateResponse struct {
	Data answerRateData `json:"data"`
}

type knowledgeGapRow struct {
	ID              int64  `json:"id" example:"7"`
	QueryText       string `json:"query_text" example:"how to reset password"`
	OccurrenceCount int64  `json:"occurrence_count" example:"14"`
	FirstSeen       string `json:"first_seen" example:"2026-07-01T09:00:00Z"`
	LastSeen        string `json:"last_seen" example:"2026-08-01T09:00:00Z"`
	Status          string `json:"status" example:"open"`
}

type knowledgeGapsData struct {
	Gaps []knowledgeGapRow `json:"gaps"`
}

type knowledgeGapsResponse struct {
	Data knowledgeGapsData `json:"data"`
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
// @Param X-Timeout-Ms header string false "Request budget in milliseconds; Go runs within this minus headroom, falling back to server config when absent"
// @Param request body dto.ReplyRequest true "Reply request"
// @Success 200 {object} response.Envelope{data=dto.ReplyResponse}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/reply [post]
func createReply() {}

// @Summary Create chat turn
// @Description Runs the tenant-scoped chat workflow. Set debug=true to include stage_timings_ms for fast_guard, router, cache, knowledge, render, resolve, generate, parse, and store_cache.
// @Tags chat
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param X-Timeout-Ms header string false "Request budget in milliseconds; Go runs within this minus headroom, falling back to server config when absent"
// @Param request body dto.ChatRequest true "Chat request"
// @Success 200 {object} response.Envelope{data=dto.ChatResponse}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/chat [post]
func createChat() {}

// @Summary Create chat turn (streaming)
// @Description Server-Sent Events variant of /v1/chat. Each frame is `event: <type>\ndata: <json>`; types are start, token, tool, pending_action, done, error. Same request body and stage set as /v1/chat.
// @Tags chat
// @Accept json
// @Produce text/event-stream
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param X-Timeout-Ms header string false "Request budget in milliseconds; Go runs within this minus headroom, falling back to server config when absent"
// @Param request body dto.ChatRequest true "Chat request"
// @Success 200 {object} dto.ChatStreamEvent
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /v1/chat/stream [post]
func createChatStream() {}

// @Summary Confirm a pending chat action
// @Description Executes an action previously proposed via `pending_action` in a /v1/chat or /v1/chat/stream response. `action_id` is the 32-character id from that field.
// @Tags chat
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param request body dto.ChatConfirmRequest true "Confirm request"
// @Success 200 {object} response.Envelope{data=dto.ChatResponse}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Router /v1/chat/confirm-action [post]
func confirmChatAction() {}

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

// @Summary Approve review queue item
// @Description Promotes the review queue item into its target (symptom node, subject, or KB article).
// @Tags review-queue
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param id path int true "Review item ID"
// @Success 200 {object} reviewApproveResponse
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/admin/review-queue/{id}/approve [post]
func approveReviewQueueItem() {}

// @Summary AI accuracy report
// @Tags reports
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param since query string false "RFC3339 timestamp, or an integer number of days ago"
// @Success 200 {object} aiAccuracyResponse
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/reports/ai-accuracy [get]
func aiAccuracyReport() {}

// @Summary Answer rate report
// @Tags reports
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param since query string false "RFC3339 timestamp, or an integer number of days ago"
// @Param threshold query number false "Confidence threshold, default 0.85"
// @Success 200 {object} answerRateResponse
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/reports/answer-rate [get]
func answerRateReport() {}

// @Summary Knowledge gaps report
// @Tags reports
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param limit query int false "Max rows, default 20"
// @Success 200 {object} knowledgeGapsResponse
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/reports/knowledge-gaps [get]
func knowledgeGapsReport() {}

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
