package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

var errStreamUnavailable = errors.New("stream: unavailable")

type streamStubProvider struct {
	chunks      []string
	streamErr   error
	fallback    string
	fallbackErr error
	prompt      ports.Prompt
	streamCalls int
	genCalls    int
}

func (p *streamStubProvider) Generate(_ context.Context, prompt ports.Prompt) (ports.Completion, error) {
	p.genCalls++
	p.prompt = prompt
	if p.fallbackErr != nil {
		return ports.Completion{}, p.fallbackErr
	}
	return ports.Completion{Text: p.fallback}, nil
}

func (p *streamStubProvider) GenerateJSON(ctx context.Context, prompt ports.Prompt) (ports.Completion, error) {
	return p.Generate(ctx, prompt)
}

func (p *streamStubProvider) Stream(_ context.Context, prompt ports.Prompt) (<-chan ports.Completion, error) {
	p.streamCalls++
	p.prompt = prompt
	if p.streamErr != nil {
		return nil, p.streamErr
	}
	ch := make(chan ports.Completion, len(p.chunks))
	for _, c := range p.chunks {
		ch <- ports.Completion{Text: c, Vendor: "stub"}
	}
	close(ch)
	return ch, nil
}

func TestRunStreamForwardsDeltasThenDone(t *testing.T) {
	provider := &streamStubProvider{chunks: []string{"Hel", "lo"}}
	fts := &stubFTS{chunks: []rag.FTSChunk{{Title: "KB-1", Content: "known fix", Relevance: 7.5}}}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var events []dto.ChatStreamEvent
	emit := func(e dto.ChatStreamEvent) error {
		events = append(events, e)
		return nil
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	err = wf.RunStream(ctx, dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "robot broke"}},
		Locale:   dto.ChatLocaleEnglish,
	}, emit)
	if err != nil {
		t.Fatalf("RunStream() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %+v, want 3 (2 deltas + done)", events)
	}
	if events[0].Type != dto.ChatStreamEventDelta || events[0].Text != "Hel" {
		t.Fatalf("event[0] = %+v", events[0])
	}
	if events[1].Type != dto.ChatStreamEventDelta || events[1].Text != "lo" {
		t.Fatalf("event[1] = %+v", events[1])
	}
	last := events[2]
	if last.Type != dto.ChatStreamEventDone || last.Status != dto.ChatStatusAnswered {
		t.Fatalf("event[2] = %+v, want done/answered", last)
	}
	if len(last.Sources) != 1 || last.Sources[0].Title != "KB-1" {
		t.Fatalf("done sources = %+v, want KB-1", last.Sources)
	}
	if provider.streamCalls != 1 {
		t.Fatalf("streamCalls = %d, want 1", provider.streamCalls)
	}
	if provider.genCalls != 0 {
		t.Fatalf("genCalls = %d, want 0 when stream succeeds", provider.genCalls)
	}
}

func TestRunStreamFallsBackToGenerateWhenStreamFails(t *testing.T) {
	provider := &streamStubProvider{streamErr: errStreamUnavailable, fallback: "fallback answer"}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var events []dto.ChatStreamEvent
	emit := func(e dto.ChatStreamEvent) error {
		events = append(events, e)
		return nil
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	err = wf.RunStream(ctx, dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "hi"}},
	}, emit)
	if err != nil {
		t.Fatalf("RunStream() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v, want 1 delta + done", events)
	}
	if events[0].Type != dto.ChatStreamEventDelta || events[0].Text != "fallback answer" {
		t.Fatalf("fallback delta = %+v", events[0])
	}
	if events[1].Type != dto.ChatStreamEventDone {
		t.Fatalf("event[1] = %+v, want done", events[1])
	}
	if provider.streamCalls != 1 || provider.genCalls != 1 {
		t.Fatalf("streamCalls=%d genCalls=%d, want 1/1", provider.streamCalls, provider.genCalls)
	}
}

func TestRunStreamEmitsErrorWhenStreamAndFallbackBothFail(t *testing.T) {
	provider := &streamStubProvider{streamErr: errStreamUnavailable, fallbackErr: errStreamUnavailable}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var events []dto.ChatStreamEvent
	emit := func(e dto.ChatStreamEvent) error {
		events = append(events, e)
		return nil
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	err = wf.RunStream(ctx, dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "hi"}},
	}, emit)
	if err != nil {
		t.Fatalf("RunStream() error = %v", err)
	}
	if len(events) != 1 || events[0].Type != dto.ChatStreamEventError {
		t.Fatalf("events = %+v, want single error event", events)
	}
	if events[0].Code == "" {
		t.Fatal("error event missing code")
	}
}

func TestRunStreamOffDomainSkipsProviderStream(t *testing.T) {
	provider := &streamStubProvider{chunks: []string{"should not run"}}
	fts := &stubFTS{}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts, WithRouter(routerForTest(t, fts)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var events []dto.ChatStreamEvent
	emit := func(e dto.ChatStreamEvent) error {
		events = append(events, e)
		return nil
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 3)
	err = wf.RunStream(ctx, dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "ติดต่อ"}},
	}, emit)
	if err != nil {
		t.Fatalf("RunStream() error = %v", err)
	}
	if provider.streamCalls != 0 {
		t.Fatalf("streamCalls = %d, want 0 for handoff", provider.streamCalls)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v, want 1 delta + done", events)
	}
	if events[1].Status != dto.ChatStatusHandoff {
		t.Fatalf("status = %q, want handoff", events[1].Status)
	}
}

func TestRunStreamRequiresCompanyScope(t *testing.T) {
	wf, err := New(prompts.MustNewRegistry(), resolverFor(&streamStubProvider{}), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = wf.RunStream(context.Background(), dto.ChatRequest{
		Messages: []dto.ChatMessage{{Role: dto.ChatRoleUser, Content: "hi"}},
	}, func(dto.ChatStreamEvent) error { return nil })
	if err == nil {
		t.Fatal("RunStream() error = nil, want company scope error")
	}
}
