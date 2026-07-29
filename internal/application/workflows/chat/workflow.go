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
	resolve     rag.ProviderForCompany
	fts         FTSSource
	profile     ProfileSource
	cases       CaseSource
	router      *intent.Router
	orch        toolRunner
	cache       ports.Cache
	cacheTTL    time.Duration
	fetcher     ports.AttachmentFetcher
	transcriber ports.Transcriber
}

func New(registry *prompts.Registry, resolve rag.ProviderForCompany, fts FTSSource, opts ...Option) (*Workflow, error) {
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
	w := &Workflow{tmpl: tmpl, streamTmpl: streamTmpl, resolve: resolve, fts: fts}
	for _, opt := range opts {
		opt(w)
	}
	return w, nil
}

func (w *Workflow) Run(ctx context.Context, req dto.ChatRequest) (dto.ChatResponse, error) {
	if err := req.Validate(); err != nil {
		return dto.ChatResponse{}, err
	}
	timings := chatTimings(req.Debug)

	pre, err := w.prepare(ctx, req, timings)
	if err != nil {
		return dto.ChatResponse{}, err
	}
	if pre.handled {
		resp := pre.resp
		resp.Transcript = pre.transcript
		return withChatTimings(resp, timings), nil
	}
	companyID, lastUser := pre.companyID, pre.lastUser

	principal, _ := ctxkey.PrincipalFrom(ctx)
	key := cacheKey(companyID, principal, req)
	var (
		cachedResp dto.ChatResponse
		cachedOK   bool
	)
	timedChat(timings, "cache", func() {
		cachedResp, cachedOK = w.cachedResponse(ctx, key)
	})
	if cachedOK {
		return withChatTimings(cachedResp, timings), nil
	}

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

	turn, err := w.generateTurn(ctx, req, companyID, knowledge, profileBlock, timings)
	if err != nil {
		return dto.ChatResponse{}, err
	}

	resp := buildChatResponse(req, pre.transcript, turn, sources)
	w.applyCaseSearch(ctx, req.Locale, companyID, turn.Search, &resp, timings)
	timedChat(timings, "store_cache", func() {
		w.storeResponse(ctx, key, resp)
	})
	return withChatTimings(resp, timings), nil
}

func offDomainReply(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "I can only help with support cases, knowledge references, case status, and staff handoff for this service."
	}
	return "ฉันช่วยได้เฉพาะเรื่องเคสบริการ แหล่งอ้างอิง สถานะเคส และการติดต่อเจ้าหน้าที่ในระบบนี้"
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

func toolFailedReply(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "I could not complete that lookup. Please try again."
	}
	return "ไม่สามารถดำเนินการค้นหาได้ กรุณาลองใหม่อีกครั้ง"
}
