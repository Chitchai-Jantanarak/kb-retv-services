package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
)

type stubFetcher struct {
	data ports.AttachmentData
	err  error
	att  ports.Attachment
}

func (s *stubFetcher) Fetch(_ context.Context, att ports.Attachment) (ports.AttachmentData, error) {
	s.att = att
	return s.data, s.err
}

type stubTranscriber struct {
	text  string
	err   error
	input ports.AudioInput
}

func (s *stubTranscriber) Transcribe(_ context.Context, in ports.AudioInput) (ports.Transcript, error) {
	s.input = in
	return ports.Transcript{Text: s.text}, s.err
}

func audioOnlyRequest() dto.ChatRequest {
	return dto.ChatRequest{
		Messages: []dto.ChatMessage{{
			Role: dto.ChatRoleUser,
			Attachments: []dto.AttachmentRef{
				{ID: "att-1", MIMEType: "audio/webm", URL: "https://cdn.example.com/audio.webm"},
			},
		}},
	}
}

func TestRunAudioOnlyMessageTranscribesAndRoutesOnTranscript(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"i cannot help with that","case":null}`}
	fts := &stubFTS{}
	fetcher := &stubFetcher{data: ports.AttachmentData{Bytes: []byte("audio-bytes"), MIMEType: "audio/webm"}}
	transcriber := &stubTranscriber{text: "tell me a joke"}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), fts,
		WithRouter(routerForTest(t, fts)),
		WithTranscription(fetcher, transcriber),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	resp, err := wf.Run(ctx, audioOnlyRequest())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.Transcript != "tell me a joke" {
		t.Fatalf("Transcript = %q, want %q", resp.Transcript, "tell me a joke")
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1: an out-of-scope ask is answered by the model, not refused by a guard", provider.calls)
	}
	if !strings.Contains(provider.prompt.User, "tell me a joke") {
		t.Fatalf("prompt = %q, want the transcript routed into it", provider.prompt.User)
	}
}

func TestRunTextPlusAudioAppendsQuotedTranscriptToPrompt(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"ok","case":null}`}
	fetcher := &stubFetcher{data: ports.AttachmentData{Bytes: []byte("audio-bytes"), MIMEType: "audio/webm"}}
	transcriber := &stubTranscriber{text: "the printer beeps three times"}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil, WithTranscription(fetcher, transcriber))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	_, err = wf.Run(ctx, dto.ChatRequest{
		Messages: []dto.ChatMessage{{
			Role:    dto.ChatRoleUser,
			Content: "my printer is broken",
			Attachments: []dto.AttachmentRef{
				{ID: "att-1", MIMEType: "audio/webm", URL: "https://cdn.example.com/audio.webm"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := "my printer is broken\n\n> the printer beeps three times"
	if !strings.Contains(provider.prompt.User, want) {
		t.Fatalf("prompt.User = %q, want it to contain %q", provider.prompt.User, want)
	}
}

func TestRunTranscriberErrorFailsRun(t *testing.T) {
	provider := &stubProvider{text: `{"reply":"ok","case":null}`}
	fetcher := &stubFetcher{data: ports.AttachmentData{Bytes: []byte("audio-bytes"), MIMEType: "audio/webm"}}
	transcriber := &stubTranscriber{err: errors.New("transcribe: boom")}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil, WithTranscription(fetcher, transcriber))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	_, err = wf.Run(ctx, audioOnlyRequest())
	if err == nil {
		t.Fatal("Run() error = nil, want transcriber error")
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func TestRunAudioOnlyWithoutTranscriberRequiresUserMessage(t *testing.T) {
	wf, err := New(prompts.MustNewRegistry(), resolverFor(&stubProvider{}), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	_, err = wf.Run(ctx, audioOnlyRequest())
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeInvalidInput {
		t.Fatalf("Run() error = %v, want code %s", err, apperr.CodeInvalidInput)
	}
}

func TestRunStreamEmitsTranscriptEventBeforeDeltas(t *testing.T) {
	provider := &streamStubProvider{chunks: []string{"Hi"}}
	fetcher := &stubFetcher{data: ports.AttachmentData{Bytes: []byte("audio-bytes"), MIMEType: "audio/webm"}}
	transcriber := &stubTranscriber{text: "hello there"}
	wf, err := New(prompts.MustNewRegistry(), resolverFor(provider), nil, WithTranscription(fetcher, transcriber))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var events []dto.ChatStreamEvent
	emit := func(e dto.ChatStreamEvent) error {
		events = append(events, e)
		return nil
	}

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	err = wf.RunStream(ctx, audioOnlyRequest(), emit)
	if err != nil {
		t.Fatalf("RunStream() error = %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("events = %+v, want at least transcript + delta/done", events)
	}
	if events[0].Type != dto.ChatStreamEventTranscript || events[0].Text != "hello there" {
		t.Fatalf("event[0] = %+v, want transcript event", events[0])
	}
	last := events[len(events)-1]
	if last.Type != dto.ChatStreamEventDone || last.Transcript != "hello there" {
		t.Fatalf("last event = %+v, want done with transcript", last)
	}
}
