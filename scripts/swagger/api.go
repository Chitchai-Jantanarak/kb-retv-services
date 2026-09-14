package main

import (
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/transport/http/response"
)

var (
	_ = dto.ChatRequest{}
	_ = dto.ChatResponse{}
	_ = dto.ReplyRequest{}
	_ = dto.ChatConfirmRequest{}
	_ = dto.ChatStreamEvent{}
	_ = dto.SearchRequest{}
	_ = dto.SearchResponse{}
	_ = response.Envelope{}
	_ = llm.TaskModels{}
	_ = llm.CatalogSnapshot{}
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

// @Summary Semantic search over reports and knowledge articles
// @Description Embeds the query, runs ANN search over the tenant's vector collection, and resolves hits back to reports. When types includes "kb" and Memgraph is enabled, each hit's classification symptom is used as a graph anchor to pull related knowledge articles. graph_anchors reports how many distinct symptoms were resolved: zero means the tenant has no classification rows, so the graph leg cannot return anything.
// @Tags search
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param X-Timeout-Ms header string false "Request budget in milliseconds; Go runs within this minus headroom, falling back to server config when absent"
// @Param request body dto.SearchRequest true "Search request; types defaults to [report, kb], limit 1-50"
// @Success 200 {object} response.Envelope{data=dto.SearchResponse}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/search [post]
func createSearch() {}

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

type aiUsageTotals struct {
	Requests     int64 `json:"requests" example:"1200"`
	Errors       int64 `json:"errors" example:"14"`
	InputTokens  int64 `json:"input_tokens" example:"450000"`
	OutputTokens int64 `json:"output_tokens" example:"98000"`
	LatencyAvgMS int64 `json:"latency_avg_ms" example:"850"`
	LatencyP50MS int64 `json:"latency_p50_ms" example:"700"`
	LatencyP95MS int64 `json:"latency_p95_ms" example:"2100"`
}

type aiUsageToday struct {
	Requests     int64 `json:"requests" example:"80"`
	Errors       int64 `json:"errors" example:"1"`
	InputTokens  int64 `json:"input_tokens" example:"30000"`
	OutputTokens int64 `json:"output_tokens" example:"6200"`
}

type aiUsageDay struct {
	Date         string         `json:"date" example:"2026-09-07"`
	Requests     int64          `json:"requests" example:"150"`
	Errors       int64          `json:"errors" example:"2"`
	InputTokens  int64          `json:"input_tokens" example:"56000"`
	OutputTokens int64          `json:"output_tokens" example:"12000"`
	ByVendor     map[string]int `json:"by_vendor"`
	ByStatus     map[string]int `json:"by_status"`
}

type aiUsageModel struct {
	Vendor       string `json:"vendor" example:"google"`
	Model        string `json:"model" example:"gemini-2.5-flash"`
	Requests     int64  `json:"requests" example:"600"`
	Errors       int64  `json:"errors" example:"5"`
	InputTokens  int64  `json:"input_tokens" example:"220000"`
	OutputTokens int64  `json:"output_tokens" example:"45000"`
	LatencyAvgMS int64  `json:"latency_avg_ms" example:"820"`
}

type aiUsageModelDay struct {
	Date         string `json:"date" example:"2026-09-07"`
	Requests     int64  `json:"requests" example:"80"`
	Errors       int64  `json:"errors" example:"1"`
	InputTokens  int64  `json:"input_tokens" example:"29000"`
	OutputTokens int64  `json:"output_tokens" example:"6000"`
}

type aiUsageModelDaily struct {
	Vendor string            `json:"vendor" example:"google"`
	Model  string            `json:"model" example:"gemini-2.5-flash"`
	Daily  []aiUsageModelDay `json:"daily"`
}

type aiUsageRecentRow struct {
	Timestamp    string `json:"ts" example:"2026-09-07T10:15:00Z"`
	Vendor       string `json:"vendor" example:"google"`
	Model        string `json:"model" example:"gemini-2.5-flash"`
	Op           string `json:"op" example:"chat"`
	Status       string `json:"status" example:"ok"`
	HTTPStatus   int    `json:"http_status" example:"200"`
	LatencyMs    int    `json:"latency_ms" example:"780"`
	InputTokens  int    `json:"input_tokens" example:"512"`
	OutputTokens int    `json:"output_tokens" example:"128"`
	Error        string `json:"error,omitempty" example:""`
}

type aiUsageData struct {
	Days        int                 `json:"days" example:"7"`
	Totals      aiUsageTotals       `json:"totals"`
	Today       aiUsageToday        `json:"today"`
	Daily       []aiUsageDay        `json:"daily"`
	ByModel     []aiUsageModel      `json:"by_model"`
	ModelsDaily []aiUsageModelDaily `json:"models_daily"`
	Recent      []aiUsageRecentRow  `json:"recent"`
}

type aiUsageResponse struct {
	Data aiUsageData `json:"data"`
}

type intakeAssessRequest struct {
	ConversationID int64 `json:"conversation_id" example:"501"`
}

type intakeAssessData struct {
	ConversationID int64  `json:"conversation_id" example:"501"`
	MessageID      int64  `json:"message_id" example:"9021"`
	Status         string `json:"status" example:"queued"`
}

type intakeAssessResponse struct {
	Data intakeAssessData `json:"data"`
}

type knowledgeStatsMemgraphLabel struct {
	Label string `json:"label" example:"symptom"`
	Count int    `json:"count" example:"128"`
}

type knowledgeStatsMemgraphEdgeType struct {
	Type  string `json:"type" example:"RELATES_TO"`
	Count int    `json:"count" example:"64"`
}

type knowledgeStatsMemgraph struct {
	Available bool                             `json:"available" example:"true"`
	Nodes     int                              `json:"nodes" example:"512"`
	Edges     int                              `json:"edges" example:"340"`
	Labels    []knowledgeStatsMemgraphLabel    `json:"labels"`
	EdgeTypes []knowledgeStatsMemgraphEdgeType `json:"edge_types"`
}

type knowledgeStatsQdrant struct {
	Available  bool   `json:"available" example:"true"`
	Collection string `json:"collection" example:"tenant_4_kb"`
	Vectors    int    `json:"vectors" example:"3400"`
	Dim        int    `json:"dim" example:"1536"`
}

type knowledgeStatsData struct {
	Memgraph knowledgeStatsMemgraph `json:"memgraph"`
	Qdrant   knowledgeStatsQdrant   `json:"qdrant"`
}

type knowledgeStatsResponse struct {
	Data knowledgeStatsData `json:"data"`
}

type knowledgeGraphNode struct {
	ID     string `json:"id" example:"symptom:123"`
	Label  string `json:"label" example:"Slow internet"`
	Type   string `json:"type" example:"Symptom"`
	Degree int    `json:"degree" example:"6"`
}

type knowledgeGraphEdge struct {
	From string `json:"from" example:"symptom:123"`
	To   string `json:"to" example:"subject:45"`
	Type string `json:"type" example:"RELATES_TO"`
}

type knowledgeGraphData struct {
	Available bool                 `json:"available" example:"true"`
	Nodes     []knowledgeGraphNode `json:"nodes"`
	Edges     []knowledgeGraphEdge `json:"edges"`
}

type knowledgeGraphResponse struct {
	Data knowledgeGraphData `json:"data"`
}

// @Summary AI usage report
// @Description Requires ai:reports:read. Aggregates request counts, token usage, and latency over the trailing window.
// @Tags reports
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param days query int false "Window size in days, clamped 1-30, default 7"
// @Success 200 {object} aiUsageResponse
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/ai/usage [get]
func aiUsageReport() {}

// @Summary List chat task models
// @Description Requires ai:reply:create and feature.ai.enabled. Returns the models configured for the tenant's "chat" AI route.
// @Tags chat
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Success 200 {object} response.Envelope{data=llm.TaskModels}
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/chat/models [get]
func listChatModels() {}

// @Summary List models for an AI connection
// @Description Requires activity.view. Returns the cached model catalog for the given AI connection.
// @Tags ai-connections
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param id path int true "AI connection ID"
// @Success 200 {object} response.Envelope{data=llm.CatalogSnapshot}
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/ai/connections/{id}/models [get]
func listAIConnectionModels() {}

// @Summary Refresh models for an AI connection
// @Description Requires activity.view. Re-discovers the provider's available models and replaces the cached catalog.
// @Tags ai-connections
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param id path int true "AI connection ID"
// @Success 200 {object} response.Envelope{data=llm.CatalogSnapshot}
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/ai/connections/{id}/models/refresh [post]
func refreshAIConnectionModels() {}

// @Summary Manually trigger AI draft assessment
// @Description Requires ai:reply:create and feature.ai.enabled. Enqueues an AI draft for a conversation's pending inbound message.
// @Tags reply
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param request body intakeAssessRequest true "Manual intake assessment request"
// @Success 202 {object} intakeAssessResponse
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/intake/assess [post]
func createIntakeAssess() {}

// @Summary Knowledge base stats
// @Description Requires ai:reports:read. Reports Memgraph node/edge counts and Qdrant vector collection size for the tenant.
// @Tags reports
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Success 200 {object} knowledgeStatsResponse
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/knowledge/stats [get]
func knowledgeStats() {}

// @Summary Knowledge graph snapshot
// @Description Requires ai:reports:read. Returns the tenant's top nodes by degree and the edges between them, for graph visualization.
// @Tags reports
// @Produce json
// @Param Authorization header string true "Bearer <RS256 service JWT>"
// @Param X-Tenant-Id header string true "Tenant ID"
// @Param limit query int false "Max nodes, clamped 10-200, default 80"
// @Success 200 {object} knowledgeGraphResponse
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Failure 500 {object} response.Envelope
// @Router /v1/knowledge/graph [get]
func knowledgeGraph() {}
