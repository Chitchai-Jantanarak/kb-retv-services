package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/intent"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/application/tools"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/memcache"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
)

type stubProvider struct {
	text   string
	prompt ports.Prompt
	calls  int
}

func (p *stubProvider) Generate(context.Context, ports.Prompt) (ports.Completion, error) {
	return ports.Completion{Text: p.text}, nil
}

func (p *stubProvider) GenerateJSON(_ context.Context, prompt ports.Prompt) (ports.Completion, error) {
	p.prompt = prompt
	p.calls++
	return ports.Completion{Text: p.text}, nil
}

func (p *stubProvider) Stream(context.Context, ports.Prompt) (<-chan ports.Completion, error) {
	ch := make(chan ports.Completion)
	close(ch)
	return ch, nil
}

type stubFTS struct {
	chunks []rag.FTSChunk
}

func (s *stubFTS) SearchChunks(context.Context, []int64, string, int) ([]rag.FTSChunk, error) {
	return s.chunks, nil
}

func resolverFor(provider ports.LLMProvider) rag.ProviderForCompany {
	return func(context.Context, int64) (ports.LLMProvider, error) {
		return provider, nil
	}
}

func privilegedChatContext(companyID int64) context.Context {
	ctx := ctxkey.WithCompanyID(context.Background(), companyID)
	return ctxkey.WithPrincipal(ctx, ctxkey.Principal{
		CompanyID: companyID,
		UserID:    9,
		Perms:     []string{chatDebugPermission},
	})
}

func TestNewRequiresRegistryAndResolver(t *testing.T) {
	if _, err := New(nil, resolverFor(&stubProvider{}), nil); err == nil {
		t.Fatal("New without registry error = nil, want error")
	}
	if _, err := New(prompts.MustNewRegistry(), nil, nil); err == nil {
		t.Fatal("New without resolver error = nil, want error")
	}
}

func TestRunRequiresCompanyScope(t *testing.T) {
	wf, err := New(prompts.MustNewRegistry(), resolverFor(&stubProvider{}), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = wf.Run(context.Background(), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "hi"}},
	})
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeUnauthorized {
		t.Fatalf("Run() error = %v, want code %s", err, apperr.CodeUnauthorized)
	}
}

func TestRunRequiresUserMessage(t *testing.T) {
	wf, err := New(prompts.MustNewRegistry(), resolverFor(&stubProvider{}), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	_, err = wf.Run(ctx, dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleAssistant, Content: "hello"}},
	})
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeInvalidInput {
		t.Fatalf("Run() error = %v, want code %s", err, apperr.CodeInvalidInput)
	}
}

func TestRunReturnsReplySourcesAndCase(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"summary","case":{"problem":"p","detail":"d","product":"x","ready":true}}`}
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "KB-1", Content: "known fix", Relevance: 7.5}}}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	resp, err := wf.Run(ctx, dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "robot broke"}},
		Locale:   dto.ChatLocaleEnglish,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.Reply != "summary" {
		t.Fatalf("Reply = %q, want summary", resp.Reply)
	}
	if len(resp.Sources) != 1 || resp.Sources[0].Title != "KB-1" {
		t.Fatalf("Sources = %v, want [KB-1]", resp.Sources)
	}
	if resp.Case == nil || !resp.Case.Ready {
		t.Fatalf("Case = %+v, want ready draft", resp.Case)
	}
	if provider.prompt.MaxToks != 1400 {
		t.Fatalf("MaxToks = %d, want 1400 from chat template", provider.prompt.MaxToks)
	}
	if !strings.Contains(provider.prompt.System, "English") {
		t.Fatal("System prompt missing rendered language")
	}
	if strings.Contains(provider.prompt.System, "known fix") {
		t.Fatal("retrieved content reached the system slot, where it reads as instruction")
	}
	if !strings.Contains(provider.prompt.User, "known fix") {
		t.Fatal("User prompt missing rendered knowledge block")
	}
	if !strings.Contains(provider.prompt.User, "[BEGIN REFERENCE NOTES") {
		t.Fatal("knowledge block missing untrusted-data delimiters")
	}
	if !strings.Contains(provider.prompt.User, "User: robot broke") {
		t.Fatal("User prompt missing transcript")
	}
}

func TestRunPassesLastUserAttachmentsToPrompt(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"ok","case":null}`}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	_, err = wf.Run(ctx, dto.ChatRequest{
		Messages: []dto.ChatMessage{
			{Role: dto.ChatRoleUser, Content: "here is a photo", Attachments: []dto.AttachmentRef{
				{ID: "att-1", MIMEType: "image/png", StorageKey: "s3://bucket/key.png", URL: "https://cdn.example.com/key.png", SizeBytes: 1024},
			}},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(provider.prompt.Attachments) != 1 {
		t.Fatalf("prompt.Attachments = %+v, want 1 converted attachment", provider.prompt.Attachments)
	}
	got := provider.prompt.Attachments[0]
	want := ports.Attachment{ID: "att-1", MIMEType: "image/png", StorageKey: "s3://bucket/key.png", URL: "https://cdn.example.com/key.png", SizeBytes: 1024}
	if got != want {
		t.Fatalf("Attachments[0] = %+v, want %+v", got, want)
	}
}

func TestRunDebugSurfacesChatStageTimings(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"summary","case":null}`}
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "KB-1", Content: "known fix"}}}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(privilegedChatContext(7), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "robot broke"}},
		Locale:   dto.ChatLocaleEnglish,
		Debug:    true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, stage := range []string{"knowledge", "render", "resolve", "generate", "parse"} {
		if _, ok := resp.StageTimingsMS[stage]; !ok {
			t.Fatalf("stage %q missing from timings: %+v", stage, resp.StageTimingsMS)
		}
	}
}

func TestRunRejectsDebugForUnprivilegedPrincipal(t *testing.T) {
	provider := &stubProvider{text: dto.ChatStatusAnswered}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	ctx = ctxkey.WithPrincipal(ctx, ctxkey.Principal{CompanyID: 7, UserID: 3, Perms: []string{string('*')}})
	resp, err := wf.Run(ctx, dto.ChatRequest{Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: dto.ChatStatusAnswered}}, Debug: true})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StageTimingsMS != nil {
		t.Fail()
	}
	if resp.Debug != nil {
		t.Fail()
	}
}

func TestRunOmitsChatStageTimingsWithoutDebug(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"summary","case":null}`}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 7), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "robot broke"}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.StageTimingsMS != nil {
		t.Fatalf("StageTimingsMS = %+v, want nil without debug", resp.StageTimingsMS)
	}
}

func TestParseTurn(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantReply string
		wantCase  bool
	}{
		{
			name:      "clean json",
			raw:       `{"reply":"hello","case":{"problem":"p","detail":"d","product":"x","ready":false}}`,
			wantReply: "hello",
			wantCase:  true,
		},
		{
			name:      "fenced json",
			raw:       "```json\n{\"reply\":\"hello\",\"case\":null}\n```",
			wantReply: "hello",
		},
		{
			name:      "json embedded in prose",
			raw:       `Here you go: {"reply":"hello","case":null} hope that helps`,
			wantReply: "hello",
		},
		{
			name:      "regex fallback on broken json",
			raw:       `{"reply":"hello \"there\"","case":{broken`,
			wantReply: `hello "there"`,
		},
		{
			name:      "plain text fallback",
			raw:       "  just text  ",
			wantReply: "just text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			turn, _ := parseTurn(tt.raw)
			if turn.Reply != tt.wantReply {
				t.Fatalf("Reply = %q, want %q", turn.Reply, tt.wantReply)
			}
			if (turn.Case != nil) != tt.wantCase {
				t.Fatalf("Case = %+v, want present=%v", turn.Case, tt.wantCase)
			}
		})
	}
}

func TestNormalizeCaseDraft(t *testing.T) {
	if got := normalizeCaseDraft(nil); got != nil {
		t.Fatalf("normalizeCaseDraft(nil) = %+v, want nil", got)
	}
	if got := normalizeCaseDraft(&dto.ChatCaseDraft{Problem: " ", Detail: "", Product: "  "}); got != nil {
		t.Fatalf("empty draft = %+v, want nil", got)
	}

	draft := normalizeCaseDraft(&dto.ChatCaseDraft{Problem: " p ", Detail: "d", Ready: true})
	if draft == nil {
		t.Fatal("draft = nil, want partial draft")
	}
	if draft.Problem != "p" {
		t.Fatalf("Problem = %q, want p", draft.Problem)
	}
	if draft.Ready {
		t.Fatal("Ready = true, want downgraded to false")
	}
}

func TestRunServesRepeatFromCache(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"cached answer","case":null}`}
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "KB", Content: "printer reset steps", Relevance: 7.5}}}
	wf, err := New(
		prompts.MustNewRegistry(),
		resolverFor(provider),
		fts,
		WithCache(memcache.New(16), time.Minute),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	req := dto.ChatRequest{Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "how to reset printer"}}}

	first, err := wf.Run(ctx, req)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	second, err := wf.Run(ctx, req)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1 (second run cached)", provider.calls)
	}
	if first.Reply != second.Reply {
		t.Fatalf("cached reply mismatch: %q vs %q", first.Reply, second.Reply)
	}
}

func TestRunDebugCacheHitSurfacesCacheTimingOnlyAfterRouter(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"cached answer","case":null}`}
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "KB", Content: "printer reset steps", Relevance: 7.5}}}
	wf, err := New(
		prompts.MustNewRegistry(),
		resolverFor(provider),
		fts,
		WithCache(memcache.New(16), time.Minute),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := privilegedChatContext(7)
	req := dto.ChatRequest{Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "how to reset printer"}}}
	if _, err := wf.Run(ctx, req); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	cached, err := wf.Run(ctx, dto.ChatRequest{
		Messages: req.Messages,
		Debug:    true,
	})
	if err != nil {
		t.Fatalf("cached Run: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if _, ok := cached.StageTimingsMS["cache"]; !ok {
		t.Fatalf("cache timing missing: %+v", cached.StageTimingsMS)
	}
	if _, ok := cached.StageTimingsMS["generate"]; ok {
		t.Fatalf("generate timing recorded on cache hit: %+v", cached.StageTimingsMS)
	}
}

func TestRunCacheIsScopedByCompanyAndTranscript(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"answer","case":null}`}
	wf, err := New(
		prompts.MustNewRegistry(),
		resolverFor(provider),
		nil,
		WithCache(memcache.New(16), time.Minute),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := dto.ChatRequest{Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "hello"}}}
	if _, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 7), req); err != nil {
		t.Fatalf("company 7: %v", err)
	}
	if _, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 8), req); err != nil {
		t.Fatalf("company 8: %v", err)
	}
	other := dto.ChatRequest{Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "different question"}}}
	if _, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 7), other); err != nil {
		t.Fatalf("other transcript: %v", err)
	}
	if provider.calls != 3 {
		t.Fatalf("provider calls = %d, want 3 (no cross-company or cross-transcript hits)", provider.calls)
	}
}

func TestRunWithoutCacheCallsProviderEveryTime(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"answer","case":null}`}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	req := dto.ChatRequest{Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "hello"}}}
	for i := range 2 {
		if _, err := wf.Run(ctx, req); err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want 2", provider.calls)
	}
}

func TestRunDoesNotCacheEmptyReply(t *testing.T) {
	provider := &stubProvider{text: "   "}
	wf, err := New(
		prompts.MustNewRegistry(),
		resolverFor(provider),
		nil,
		WithCache(memcache.New(16), time.Minute),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	req := dto.ChatRequest{Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "hello"}}}
	for i := range 2 {
		if _, err := wf.Run(ctx, req); err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 (empty reply must not be cached)", provider.calls)
	}
}

func routerForTest(t *testing.T, fts rag.FTSSource) *intent.Router {
	t.Helper()
	emb := &chatStubEmbedder{vecs: map[string][]float32{
		"ติดต่อเจ้าหน้าที่": {1, 0, 0},
		"ตรวจสอบสถานะเคส":   {0, 1, 0},
		"เปิดเคสใหม่":       {0, 0, 1},
		"robot broken":      {0, 0, 0, 1},
		"ติดต่อ":            {1, 0, 0},
		"ขอสูตรอาหาร":       {0, 0, 0, 0, 1},
		"สวัสดี":            {0, 0, 0, 0, 0, 1},
	}}
	r, err := intent.New(context.Background(), emb, newFTSScorer(fts),
		intent.WithAnchors(map[intent.Intent][]string{
			intent.Handoff:    {"ติดต่อเจ้าหน้าที่"},
			intent.CaseStatus: {"ตรวจสอบสถานะเคส"},
			intent.OpenCase:   {"เปิดเคสใหม่"},
			intent.OffDomain:  {"ขอสูตรอาหาร"},
			intent.Social:     {"สวัสดี"},
		}),
		intent.WithThresholds(0.4, 0.6),
	)
	if err != nil {
		t.Fatalf("intent.New: %v", err)
	}
	return r
}

type stubOrch struct {
	resp skeleton.Response
	err  error
}

func (s stubOrch) Handle(_ context.Context, _ skeleton.Actor, _ string) (skeleton.Response, error) {
	return s.resp, s.err
}

type fakeTurnRecorder struct {
	calls  int
	audits []TurnAudit
}

func (f *fakeTurnRecorder) RecordChatTurn(_ context.Context, _ int64, audit TurnAudit) error {
	f.calls++
	f.audits = append(f.audits, audit)
	return nil
}

func TestRunToolPathRecordsExactlyOneTurn(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"should not run","case":null}`}
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "KB", Content: "x", Relevance: 7.5}}}
	orch := stubOrch{resp: skeleton.Response{Matched: true, ToolID: "f1_find_cases", Headline: "2 cases match", Lines: []string{"C1|open", "C2|done"}}}
	turns := &fakeTurnRecorder{}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)), WithOrchestrator(orch), WithTurnRecorder(turns))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "robot broken"}},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if turns.calls != 1 {
		t.Fatalf("RecordChatTurn calls = %d, want exactly 1", turns.calls)
	}
	if turns.audits[0].ToolID != "f1_find_cases" {
		t.Fatalf("audit.ToolID = %q, want f1_find_cases", turns.audits[0].ToolID)
	}
}

func TestRunGenerativePathRecordsExactlyOneTurn(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"llm answer","case":null}`}
	turns := &fakeTurnRecorder{}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil, WithTurnRecorder(turns))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 7), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "hello"}},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if turns.calls != 1 {
		t.Fatalf("RecordChatTurn calls = %d, want exactly 1", turns.calls)
	}
}

func TestRunToolPathRecordsDecisionKindToolWithScoreAndMargin(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"should not run","case":null}`}
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "KB", Content: "x", Relevance: 7.5}}}
	orch := stubOrch{resp: skeleton.Response{
		Matched:        true,
		ToolID:         "f1_find_cases",
		Headline:       "2 cases match",
		Lines:          []string{"C1|open", "C2|done"},
		SelectionScore: 0.82,
		RunnerUpScore:  0.61,
		RowCount:       2,
	}}
	turns := &fakeTurnRecorder{}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)), WithOrchestrator(orch), WithTurnRecorder(turns))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "robot broken"}},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if turns.calls != 1 {
		t.Fatalf("RecordChatTurn calls = %d, want exactly 1", turns.calls)
	}
	audit := turns.audits[0]
	if audit.DecisionKind != "tool" {
		t.Fatalf("DecisionKind = %q, want tool", audit.DecisionKind)
	}
	if audit.Score != 0.82 || audit.RunnerUpScore != 0.61 {
		t.Fatalf("Score/RunnerUpScore = %v/%v, want 0.82/0.61", audit.Score, audit.RunnerUpScore)
	}
	if audit.Margin < 0.20 || audit.Margin > 0.22 {
		t.Fatalf("Margin = %v, want ~0.21", audit.Margin)
	}
	if audit.RowsReturned != 2 {
		t.Fatalf("RowsReturned = %d, want 2", audit.RowsReturned)
	}
}

func TestDecisionKindClassifiesOffDomainAsBlocked(t *testing.T) {
	pre := chatPreamble{handled: true}
	got := decisionKindOf(dto.ChatStatusOffDomain, intent.Decision{Intent: intent.OffDomain}, pre)
	if got != "blocked" {
		t.Fatalf("decisionKindOf = %q, want blocked", got)
	}
}

func TestRunGenerativePathRecordsThaiLangAndGenerativeKind(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"llm answer","case":null}`}
	turns := &fakeTurnRecorder{}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil, WithTurnRecorder(turns))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 7), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "สวัสดีครับ ช่วยดูเคสให้หน่อย"}},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if turns.calls != 1 {
		t.Fatalf("RecordChatTurn calls = %d, want exactly 1", turns.calls)
	}
	audit := turns.audits[0]
	if audit.DecisionKind != "generative" {
		t.Fatalf("DecisionKind = %q, want generative", audit.DecisionKind)
	}
	if audit.Lang != "th" {
		t.Fatalf("Lang = %q, want th", audit.Lang)
	}
}

func TestRunToolMatchSkipsLLM(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"should not run","case":null}`}
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "KB", Content: "x", Relevance: 7.5}}}
	orch := stubOrch{resp: skeleton.Response{Matched: true, ToolID: "f1_find_cases", Headline: "2 cases match", Lines: []string{"C1|open", "C2|done"}}}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)), WithOrchestrator(orch))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "robot broken"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0 when a tool matches", provider.calls)
	}
	if resp.Status != dto.ChatStatusAnswered {
		t.Fatalf("status = %q, want answered", resp.Status)
	}
	if !strings.Contains(resp.Reply, "2 cases match") {
		t.Fatalf("reply = %q, want the tool headline", resp.Reply)
	}
}

func TestToolHitCarriesToolResult(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"unused","case":null}`}
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "KB", Content: "x", Relevance: 7.5}}}
	table := &skeleton.ToolTable{
		ToolID:  "f1_find_cases",
		Columns: []tools.ToolColumn{{Key: "code", Type: "case_ref", Primary: true}, {Key: "status", Type: "status"}},
		Rows:    []map[string]string{{"code": "REP-1", "status": "waiting"}},
	}
	orch := stubOrch{resp: skeleton.Response{Matched: true, ToolID: "f1_find_cases", Headline: "1 cases match", Lines: []string{"REP-1|waiting"}, Table: table}}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)), WithOrchestrator(orch))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "robot broken"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.ToolResult == nil {
		t.Fatal("ToolResult is nil")
	}
	if resp.ToolResult.ToolID != "f1_find_cases" || len(resp.ToolResult.Columns) != 2 || resp.ToolResult.Rows[0]["code"] != "REP-1" {
		t.Fatalf("tool result = %+v", resp.ToolResult)
	}
	if resp.Model != "" || resp.Vendor != "" {
		t.Fatalf("tool-only response must not claim an LLM model; model=%q vendor=%q", resp.Model, resp.Vendor)
	}

	var done dto.ChatStreamEvent
	err = wf.RunStream(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "robot broken"}},
	}, func(ev dto.ChatStreamEvent) error {
		if ev.Type == dto.ChatStreamEventDone {
			done = ev
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	if done.ToolResult == nil || done.ToolResult.ToolID != "f1_find_cases" {
		t.Fatalf("stream done tool result = %+v", done.ToolResult)
	}
	if done.Model != "" || done.Vendor != "" {
		t.Fatalf("tool-only stream must not claim an LLM model; model=%q vendor=%q", done.Model, done.Vendor)
	}
}

func TestRunToolNoMatchFallsThroughToLLM(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"llm answer","case":null}`}
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "KB", Content: "x", Relevance: 7.5}}}
	orch := stubOrch{resp: skeleton.Response{Matched: false}}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)), WithOrchestrator(orch))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "robot broken"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1 when no tool matches", provider.calls)
	}
	if resp.Reply != "llm answer" {
		t.Fatalf("reply = %q, want the LLM fallback", resp.Reply)
	}
}

func TestRunToolErrorFallsThroughToLLM(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"llm answer","case":null}`}
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "KB", Content: "x", Relevance: 7.5}}}
	orch := stubOrch{err: apperr.New(apperr.CodeInternal, "handler missing")}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)), WithOrchestrator(orch))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "robot broken"}},
	})
	if err != nil {
		t.Fatalf("Run must not fail when the orchestrator errors: %v", err)
	}
	if provider.calls != 1 || resp.Reply != "llm answer" {
		t.Fatalf("orchestrator error must fall through to LLM; calls=%d reply=%q", provider.calls, resp.Reply)
	}
}

type stubProfile struct {
	block string
	calls int
}

func (s *stubProfile) Build(context.Context, int64) (string, error) {
	s.calls++
	return s.block, nil
}

type stubCaseSearch struct {
	results []dto.ChatCaseResult
	err     error
	query   string
	product string
	status  string
	limit   int
	calls   int
}

func (s *stubCaseSearch) SearchCases(_ context.Context, _ []int64, query, product, status string, limit int) ([]dto.ChatCaseResult, error) {
	s.calls++
	s.query = query
	s.product = product
	s.status = status
	s.limit = limit
	if s.err != nil {
		return nil, s.err
	}
	return s.results, nil
}

func TestRunInjectsCompanyProfileIntoPrompt(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"we build robots","case":null}`}
	prof := &stubProfile{block: "You are the support assistant for Acme (acme.io).\nSupported products: Router X."}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil, WithProfile(prof))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 7), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "what do you sell?"}},
		Locale:   dto.ChatLocaleEnglish,
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if prof.calls != 1 {
		t.Fatalf("profile Build calls = %d, want 1", prof.calls)
	}
	for _, want := range []string{"Acme", "Router X"} {
		if !strings.Contains(provider.prompt.System, want) {
			t.Fatalf("system prompt missing profile %q\n%s", want, provider.prompt.System)
		}
	}
}

func TestRunExecutesSearchAndAttachesResults(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"here are matching cases","case":null,"search":{"query":"printer","status":"waiting"}}`}
	cases := &stubCaseSearch{results: []dto.ChatCaseResult{
		{ID: 9, Code: "chat:1", Title: "Printer offline", Status: "waiting"},
		{ID: 10, Code: "chat:2", Title: "Printer jam", Status: "waiting"},
	}}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil, WithCaseSearch(cases))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 7), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "find my printer cases"}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if cases.calls != 1 {
		t.Fatalf("SearchCases calls = %d, want 1", cases.calls)
	}
	if cases.query != "printer" || cases.status != "waiting" {
		t.Fatalf("search args = (%q,%q,%q), want printer/-/waiting", cases.query, cases.product, cases.status)
	}
	if len(resp.SearchResults) != 2 || resp.SearchResults[0].ID != 9 {
		t.Fatalf("SearchResults = %+v, want 2 rows starting id 9", resp.SearchResults)
	}
}

func TestRunSkipsSearchWhenTurnHasNone(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"just an answer","case":null,"search":null}`}
	cases := &stubCaseSearch{}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil, WithCaseSearch(cases))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 7), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "how do I reset it?"}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if cases.calls != 0 {
		t.Fatalf("SearchCases calls = %d, want 0 when no search in turn", cases.calls)
	}
	if len(resp.SearchResults) != 0 {
		t.Fatalf("SearchResults = %+v, want empty", resp.SearchResults)
	}
}

func TestRunSearchNoResultsSetsStatusAndReply(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"Looking up the latest incoming cases for you.","case":null,"search":{"query":"latest incoming cases"}}`}
	cases := &stubCaseSearch{}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil, WithCaseSearch(cases))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 7), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "give me 3 latest incoming cases"}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.Status != dto.ChatStatusNoResults {
		t.Fatalf("status = %q, want no_results", resp.Status)
	}
	if resp.Reply == "Looking up the latest incoming cases for you." {
		t.Fatal("reply must not surface the LLM's blind prose when the search returned zero rows")
	}
}

func TestRunSearchErrorSetsToolFailedStatus(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"here you go","case":null,"search":{"query":"printer"}}`}
	cases := &stubCaseSearch{err: apperr.New(apperr.CodeInternal, "db timeout")}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil, WithCaseSearch(cases))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 7), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "find my printer case"}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.Status != dto.ChatStatusToolFailed {
		t.Fatalf("status = %q, want tool_failed", resp.Status)
	}
}

type chatStubEmbedder struct{ vecs map[string][]float32 }

func (s *chatStubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = s.vecs[t]
	}
	return out, nil
}

func TestRunHandoffSkipsProvider(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"should not run","case":null}`}
	fts := &stubFTS{}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "ติดต่อ"}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0 for handoff", provider.calls)
	}
	if resp.Status != dto.ChatStatusHandoff {
		t.Fatalf("status = %q, want handoff", resp.Status)
	}
}

func TestRunDebugHandoffSurfacesRouterTimingAndSkipsGenerate(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"should not run","case":null}`}
	fts := &stubFTS{}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(privilegedChatContext(3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "ติดต่อ"}},
		Debug:    true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if _, ok := resp.StageTimingsMS["router"]; !ok {
		t.Fatalf("router timing missing: %+v", resp.StageTimingsMS)
	}
	if _, ok := resp.StageTimingsMS["generate"]; ok {
		t.Fatalf("generate timing recorded on handoff: %+v", resp.StageTimingsMS)
	}
}

func TestRunKBSearchReturnsStructuredSources(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"พบข้อมูลหุ่นยนต์","case":null}`}
	fts := &stubFTS{chunks: []rag.FTSChunk{{ChunkID: 9, ArticleID: 4, Title: "Robot broken case", Content: "motor failure", Relevance: 7.5}}}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "robot broken"}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1 for kb_search", provider.calls)
	}
	if resp.Status != dto.ChatStatusAnswered {
		t.Fatalf("status = %q, want answered", resp.Status)
	}
	if len(resp.Sources) != 1 || resp.Sources[0].Title != "Robot broken case" {
		t.Fatalf("sources = %+v, want one structured source", resp.Sources)
	}
	if len(resp.Activity) == 0 {
		t.Fatal("activity is empty")
	}
}

func TestRunGatesLowRelevanceKnowledge(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"hi","case":null}`}
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "unrelated report", Content: "contains the word hello", Relevance: 4.07}}}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 7), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(resp.Sources) != 0 {
		t.Fatalf("Sources = %+v, want empty for a below-floor relevance match", resp.Sources)
	}
}

func TestChatPromptForcesEvidenceUse(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"answer","case":null}`}
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "KB", Content: "robot repair steps", Relevance: 5}}}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := wf.Run(ctxkey.WithCompanyID(context.Background(), 3), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "robot broken"}},
	}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{
		"tenant-scoped support assistant",
		"Use the provided reference notes as evidence",
		"Do not tell the user to search",
		"Do not invent database records",
	} {
		if !strings.Contains(provider.prompt.System, want) {
			t.Fatalf("system prompt missing %q\n%s", want, provider.prompt.System)
		}
	}
}
