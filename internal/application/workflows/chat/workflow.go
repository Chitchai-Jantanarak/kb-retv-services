package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/intent"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
)

type toolRunner interface {
	Handle(ctx context.Context, actor skeleton.Actor, msg string) (skeleton.Response, error)
}

type FTSSource interface {
	SearchChunks(ctx context.Context, coverage []int64, query string, limit int) ([]rag.FTSChunk, error)
}

type ProfileSource interface {
	Build(ctx context.Context, companyID int64) (string, error)
}

type CaseSource interface {
	SearchCases(ctx context.Context, coverage []int64, query, product, status string, limit int) ([]dto.ChatCaseResult, error)
}

type Workflow struct {
	tmpl       prompts.Template
	streamTmpl prompts.Template
	resolve    rag.ProviderForCompany
	fts        FTSSource
	profile    ProfileSource
	cases      CaseSource
	router     *intent.Router
	orch       toolRunner
	cache      ports.Cache
	cacheTTL   time.Duration
}

type Option func(*Workflow)

func WithCache(cache ports.Cache, ttl time.Duration) Option {
	return func(w *Workflow) {
		if cache != nil && ttl > 0 {
			w.cache = cache
			w.cacheTTL = ttl
		}
	}
}

func WithRouter(router *intent.Router) Option {
	return func(w *Workflow) { w.router = router }
}

func WithOrchestrator(orch toolRunner) Option {
	return func(w *Workflow) { w.orch = orch }
}

func WithProfile(profile ProfileSource) Option {
	return func(w *Workflow) {
		if profile != nil {
			w.profile = profile
		}
	}
}

func WithCaseSearch(cases CaseSource) Option {
	return func(w *Workflow) {
		if cases != nil {
			w.cases = cases
		}
	}
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
		return withChatTimings(pre.resp, timings), nil
	}
	companyID, lastUser := pre.companyID, pre.lastUser

	key := cacheKey(companyID, req)
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

	var prompt ports.Prompt
	err = timedChatErr(timings, "render", func() error {
		var err error
		prompt, err = w.tmpl.Render(map[string]string{
			"language":   promptLanguage(req.Locale),
			"transcript": buildTranscript(req.Messages),
			"knowledge":  knowledgeSection(knowledge),
			"profile":    profileSection(profileBlock),
		})
		return err
	})
	if err != nil {
		return dto.ChatResponse{}, fmt.Errorf("chat: render prompt: %w", err)
	}
	prompt.Attachments = toPromptAttachments(lastUserAttachments(req))

	var provider ports.LLMProvider
	err = timedChatErr(timings, "resolve", func() error {
		var err error
		provider, err = w.resolve(ctx, companyID)
		return err
	})
	if err != nil {
		return dto.ChatResponse{}, fmt.Errorf("chat: resolve provider: %w", err)
	}
	var completion ports.Completion
	err = timedChatErr(timings, "generate", func() error {
		var err error
		completion, err = provider.GenerateJSON(ctx, prompt)
		return err
	})
	if err != nil {
		return dto.ChatResponse{}, err
	}

	var turn llmTurn
	var structuredOK bool
	timedChat(timings, "parse", func() {
		turn, structuredOK = parseTurn(completion.Text)
	})
	if !structuredOK && strings.TrimSpace(completion.Text) != "" {
		repair := prompt
		repair.User = prompt.User + "\n\nYour previous output was not a single JSON object. Reply again with ONLY the JSON object described above."
		if retry, rerr := provider.GenerateJSON(ctx, repair); rerr == nil {
			if rt, ok := parseTurn(retry.Text); ok {
				turn, structuredOK = rt, true
			}
		}
	}
	resp := dto.ChatResponse{
		Reply:    turn.Reply,
		Status:   dto.ChatStatusAnswered,
		Sources:  sources,
		Activity: activity(req.Locale, "request_checked", "permission_checked", "searched_knowledge", "answer_prepared"),
		Case:     normalizeCaseDraft(turn.Case),
	}
	if search := normalizeSearch(turn.Search); search != nil && w.cases != nil {
		timedChat(timings, "search", func() {
			coverage := ctxkey.CoverageOrSelf(ctx, companyID)
			if results, serr := w.cases.SearchCases(ctx, coverage, search.Query, search.Product, search.Status, chatSearchLimit); serr == nil {
				resp.SearchResults = results
			}
		})
	}
	timedChat(timings, "store_cache", func() {
		w.storeResponse(ctx, key, resp)
	})
	return withChatTimings(resp, timings), nil
}

type chatPreamble struct {
	companyID int64
	lastUser  string
	resp      dto.ChatResponse
	handled   bool
}

// prepare runs the pre-stages shared by Run and RunStream: request scope
// checks, the fast off-domain guard, intent routing, and the tool
// orchestrator short-circuit. It returns handled=true when the caller
// should use resp as the final answer without touching the LLM.
func (w *Workflow) prepare(ctx context.Context, req dto.ChatRequest, timings map[string]int64) (chatPreamble, error) {
	companyID, ok := ctxkey.CompanyID(ctx)
	if !ok || companyID <= 0 {
		return chatPreamble{}, apperr.New(apperr.CodeUnauthorized, "company scope is required")
	}
	lastUser := req.LastUserMessage()
	if lastUser == "" {
		return chatPreamble{}, apperr.New(apperr.CodeInvalidInput, "a user message is required")
	}

	if w.router == nil {
		return chatPreamble{companyID: companyID, lastUser: lastUser}, nil
	}

	fastGuardHit := false
	timedChat(timings, "fast_guard", func() {
		fastGuardHit = fastGuardOffDomain(lastUser)
	})
	if fastGuardHit {
		return chatPreamble{
			companyID: companyID,
			lastUser:  lastUser,
			handled:   true,
			resp: dto.ChatResponse{
				Reply:    offDomainReply(req.Locale),
				Status:   dto.ChatStatusOffDomain,
				Activity: activity(req.Locale, "request_checked"),
			},
		}, nil
	}

	var decision intent.Decision
	err := timedChatErr(timings, "router", func() error {
		var err error
		decision, err = w.router.Route(ctx, lastUser)
		return err
	})
	if err != nil {
		return chatPreamble{}, err
	}
	if w.orch != nil && decision.Intent != intent.OffDomain && decision.Intent != intent.Handoff {
		var toolResp skeleton.Response
		var toolErr error
		timedChat(timings, "tool", func() {
			toolResp, toolErr = w.orch.Handle(ctx, toolActor(ctx, companyID), lastUser)
		})
		if toolErr == nil && toolResp.Matched {
			return chatPreamble{
				companyID: companyID,
				lastUser:  lastUser,
				handled:   true,
				resp:      toolChatResponse(req.Locale, toolResp),
			}, nil
		}
		if authoritativeIntent(decision.Intent) {
			return chatPreamble{
				companyID: companyID,
				lastUser:  lastUser,
				handled:   true,
				resp: dto.ChatResponse{
					Reply:    handoffReply(req.Locale),
					Status:   dto.ChatStatusHandoff,
					Activity: activity(req.Locale, "request_checked", "permission_checked"),
				},
			}, nil
		}
	}
	if resp, handled := w.shortCircuit(req.Locale, decision); handled {
		return chatPreamble{companyID: companyID, lastUser: lastUser, handled: true, resp: resp}, nil
	}

	return chatPreamble{companyID: companyID, lastUser: lastUser}, nil
}

func (w *Workflow) shortCircuit(locale string, d intent.Decision) (dto.ChatResponse, bool) {
	switch d.Intent {
	case intent.OffDomain:
		return dto.ChatResponse{
			Reply:    offDomainReply(locale),
			Status:   dto.ChatStatusOffDomain,
			Activity: activity(locale, "request_checked"),
		}, true
	case intent.Handoff:
		return dto.ChatResponse{
			Reply:    handoffReply(locale),
			Status:   dto.ChatStatusHandoff,
			Activity: activity(locale, "request_checked", "permission_checked"),
		}, true
	default:
		return dto.ChatResponse{}, false
	}
}

func authoritativeIntent(i intent.Intent) bool {
	switch i {
	case intent.CaseStatus, intent.OpenCase:
		return true
	default:
		return false
	}
}

func toolActor(ctx context.Context, companyID int64) skeleton.Actor {
	principal, _ := ctxkey.PrincipalFrom(ctx)
	return skeleton.Actor{
		CompanyID: companyID,
		UserID:    principal.UserID,
		Perms:     principal.Perms,
		Coverage:  ctxkey.CoverageOrSelf(ctx, companyID),
	}
}

func toolChatResponse(locale string, r skeleton.Response) dto.ChatResponse {
	reply := r.Headline
	if len(r.Lines) > 0 {
		reply = r.Headline + "\n" + strings.Join(r.Lines, "\n")
	}
	return dto.ChatResponse{
		Reply:    reply,
		Status:   dto.ChatStatusAnswered,
		Activity: activity(locale, "request_checked", "permission_checked", "searched_knowledge", "answer_prepared"),
	}
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

func lastUserAttachments(req dto.ChatRequest) []dto.AttachmentRef {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == dto.ChatRoleUser {
			return req.Messages[i].Attachments
		}
	}
	return nil
}

func toPromptAttachments(attachments []dto.AttachmentRef) []ports.Attachment {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]ports.Attachment, len(attachments))
	for i, a := range attachments {
		out[i] = ports.Attachment{
			ID:         a.ID,
			MIMEType:   a.MIMEType,
			StorageKey: a.StorageKey,
			URL:        a.URL,
			SizeBytes:  a.SizeBytes,
		}
	}
	return out
}

func (w *Workflow) knowledgeContext(ctx context.Context, companyID int64, query string) ([]dto.ChatSource, string) {
	if w.fts == nil {
		return nil, ""
	}
	chunks, err := w.fts.SearchChunks(ctx, []int64{companyID}, query, knowledgeChunkLimit)
	if err != nil {
		return nil, ""
	}

	var (
		sources []dto.ChatSource
		block   strings.Builder
	)
	for _, chunk := range chunks {
		title := strings.TrimSpace(chunk.Title)
		snippet := strings.TrimSpace(chunk.Content)
		if len(snippet) > knowledgeSnippetMaxChars {
			snippet = snippet[:knowledgeSnippetMaxChars]
		}
		if title != "" || snippet != "" {
			sources = append(sources, dto.ChatSource{
				ID:      fmt.Sprintf("chunk:%d", chunk.ChunkID),
				Title:   title,
				Snippet: snippet,
				Source:  "fts",
				Score:   chunk.Relevance,
			})
		}
		if snippet != "" {
			fmt.Fprintf(&block, "- %s: %s\n", title, snippet)
		}
	}
	return sources, block.String()
}
