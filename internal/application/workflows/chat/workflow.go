package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/intent"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

type Workflow struct {
	tmpl        prompts.Template
	streamTmpl  prompts.Template
	summaryTmpl prompts.Template
	clarifyTmpl prompts.Template
	resolve     rag.ProviderForCompany
	fts         rag.FTSSource
	profile     ProfileSource
	cases       CaseSource
	router      *intent.Router
	orch        toolRunner
	cache       ports.Cache
	cacheTTL    time.Duration
	fetcher     ports.AttachmentFetcher
	transcriber ports.Transcriber
	sessions    SessionStore
	turns       TurnRecorder
}

func New(registry *prompts.Registry, resolve rag.ProviderForCompany, fts rag.FTSSource, opts ...Option) (*Workflow, error) {
	if registry == nil {
		return nil, fmt.Errorf("chat: prompt registry is required")
	}
	if resolve == nil {
		return nil, fmt.Errorf("chat: provider resolver is required")
	}
	tmpl, err := registry.Get(prompts.NameChat)
	if err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}
	streamTmpl, err := registry.Get(prompts.NameChatStream)
	if err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}
	summaryTmpl, err := registry.Get(prompts.NameToolSummary)
	if err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}
	clarifyTmpl, err := registry.Get(prompts.NameClarify)
	if err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}
	w := &Workflow{tmpl: tmpl, streamTmpl: streamTmpl, summaryTmpl: summaryTmpl, clarifyTmpl: clarifyTmpl, resolve: resolve, fts: fts}
	for _, opt := range opts {
		opt(w)
	}
	return w, nil
}

func (w *Workflow) Run(ctx context.Context, req dto.ChatRequest) (dto.ChatResponse, error) {
	if err := req.Validate(); err != nil {
		return dto.ChatResponse{}, err
	}
	principal, _ := ctxkey.PrincipalFrom(ctx)
	req.Debug = authorizeChatDebug(principal, req.Debug)
	req.Messages = append([]dto.ChatMessage(nil), req.Messages...)
	timings := chatTimings(req.Debug)
	started := time.Now()

	pre, err := w.prepare(ctx, req, timings)
	if err != nil {
		return dto.ChatResponse{}, err
	}
	if pre.handled {
		resp := pre.resp
		resp.Transcript = pre.transcript
		w.persistTurns(ctx, pre.companyID, req, &resp, pre.tool)
		w.recordTurn(ctx, pre.companyID, resp, pre.decision, TurnAudit{
			ToolID:       toolIDOf(pre.tool),
			ResolvedFrom: resolvedFrom(paramsOf(pre.tool), pre.candidates),
			LatencyMS:    int(time.Since(started).Milliseconds()),
		})
		resp = withChatTimings(resp, timings)
		return attachChatDebug(resp, pre.toolDebug, timings, false), nil
	}
	companyID, lastUser := pre.companyID, pre.lastUser

	key := cacheKey(companyID, principal, req)
	var (
		cachedResp dto.ChatResponse
		cachedOK   bool
	)
	timedChat(timings, "cache", func() {
		cachedResp, cachedOK = w.cachedResponse(ctx, key)
	})
	if cachedOK {
		cachedResp = withChatTimings(cachedResp, timings)
		return attachChatDebug(cachedResp, pre.toolDebug, timings, true), nil
	}

	sources, knowledge, profileBlock := w.fetchContext(ctx, companyID, lastUser, timings)

	turn, err := w.generateTurn(ctx, req, companyID, knowledge, profileBlock, timings)
	if err != nil {
		return dto.ChatResponse{}, err
	}

	resp := buildChatResponse(req, pre.transcript, turn, sources)
	w.applyCaseSearch(ctx, req.Locale, companyID, turn.Search, &resp, timings)
	w.persistTurns(ctx, companyID, req, &resp, pre.tool)
	w.recordTurn(ctx, companyID, resp, pre.decision, TurnAudit{
		ToolID:       toolIDOf(pre.tool),
		ResolvedFrom: resolvedFrom(paramsOf(pre.tool), pre.candidates),
		LatencyMS:    int(time.Since(started).Milliseconds()),
		TokensIn:     turn.TokensIn,
		TokensOut:    turn.TokensOut,
	})
	timedChat(timings, "store_cache", func() {
		w.storeResponse(ctx, key, resp)
	})
	resp = withChatTimings(resp, timings)
	return attachChatDebug(resp, pre.toolDebug, timings, false), nil
}

func (w *Workflow) fetchContext(ctx context.Context, companyID int64, lastUser string, timings map[string]int64) ([]dto.ChatSource, string, string) {
	var (
		sources   []dto.ChatSource
		knowledge string
	)
	timedChat(timings, "knowledge", func() {
		sources, knowledge = w.knowledgeContext(ctx, companyID, lastUser)
	})

	var profileBlock string
	if w.profile != nil {
		timedChat(timings, "profile", func() {
			if block, perr := w.profile.Build(ctx, companyID); perr == nil {
				profileBlock = block
			}
		})
	}
	return sources, knowledge, profileBlock
}

func offDomainReply(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "I can only help with support cases, knowledge references, case status, and staff handoff for this service."
	}
	return "ฉันช่วยได้เฉพาะเรื่องเคสบริการ แหล่งอ้างอิง สถานะเคส และการติดต่อเจ้าหน้าที่ในระบบนี้"
}

func socialReply(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "Hello. I can look up support cases, case status, team workload, products, customers, and knowledge articles. What would you like to check?"
	}
	return "สวัสดีครับ ผมช่วยดูเคสบริการ สถานะเคส ภาระงานทีม สินค้า ลูกค้า และคลังความรู้ได้ ต้องการให้ช่วยเรื่องไหนครับ"
}

func handoffReply(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "I am routing you to a staff member. Please describe your issue and they will follow up."
	}
	return "กำลังส่งต่อให้เจ้าหน้าที่ กรุณาอธิบายปัญหาของคุณ แล้วเจ้าหน้าที่จะติดต่อกลับ"
}

func noResultsReply(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "I could not find any cases matching that."
	}
	return "ไม่พบเคสที่ตรงกับที่ค้นหา"
}

func confirmActionReply(locale, summary string) string {
	if locale == dto.ChatLocaleEnglish {
		return "Please confirm this action: " + summary
	}
	return "กรุณายืนยันการดำเนินการ: " + summary
}

func permissionDeniedReply(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "You do not have permission for that lookup. Ask an administrator for access."
	}
	return "คุณไม่มีสิทธิ์ใช้งานคำสั่งนี้ กรุณาติดต่อผู้ดูแลระบบ"
}

func clarifyReply(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "I could not tell which case, employee, or product you meant. Please include the case code or the exact name."
	}
	return "ไม่แน่ใจว่าหมายถึงเคส พนักงาน หรือสินค้าใด กรุณาระบุรหัสเคสหรือชื่อที่ชัดเจน"
}

func toolFailedReply(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "I could not complete that lookup. Please try again."
	}
	return "ไม่สามารถดำเนินการค้นหาได้ กรุณาลองใหม่อีกครั้ง"
}
