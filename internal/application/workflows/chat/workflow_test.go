package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/dto"
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
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "KB-1", Content: "known fix"}}}
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
	if len(resp.Sources) != 1 || resp.Sources[0] != "KB-1" {
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
	if !strings.Contains(provider.prompt.System, "known fix") {
		t.Fatal("System prompt missing rendered knowledge block")
	}
	if !strings.Contains(provider.prompt.User, "User: robot broke") {
		t.Fatal("User prompt missing transcript")
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
			turn := parseTurn(tt.raw)
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
	for i := 0; i < 2; i++ {
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
	for i := 0; i < 2; i++ {
		if _, err := wf.Run(ctx, req); err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
	}
	if provider.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 (empty reply must not be cached)", provider.calls)
	}
}
