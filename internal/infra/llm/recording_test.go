package llm

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/llmerr"
	"github.com/my/app/internal/shared/ctxkey"
)

type recordingFakeProvider struct {
	genOut    ports.Completion
	genErr    error
	jsonOut   ports.Completion
	jsonErr   error
	streamCh  chan ports.Completion
	streamErr error
}

func (f *recordingFakeProvider) Generate(_ context.Context, _ ports.Prompt) (ports.Completion, error) {
	return f.genOut, f.genErr
}

func (f *recordingFakeProvider) GenerateJSON(_ context.Context, _ ports.Prompt) (ports.Completion, error) {
	return f.jsonOut, f.jsonErr
}

func (f *recordingFakeProvider) Stream(_ context.Context, _ ports.Prompt) (<-chan ports.Completion, error) {
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	return f.streamCh, nil
}

type fakeSink struct {
	mu      sync.Mutex
	records []RequestLog
	delay   time.Duration
	done    chan struct{}
}

func newFakeSink(expect int) *fakeSink {
	return &fakeSink{done: make(chan struct{}, expect)}
}

func (s *fakeSink) Record(_ context.Context, l RequestLog) error {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	s.mu.Lock()
	s.records = append(s.records, l)
	s.mu.Unlock()
	s.done <- struct{}{}
	return nil
}

func (s *fakeSink) wait(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-s.done:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for record %d/%d", i+1, n)
		}
	}
}

func (s *fakeSink) get() []RequestLog {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RequestLog, len(s.records))
	copy(out, s.records)
	return out
}

func TestRecordingGenerateRecordsOk(t *testing.T) {
	sink := newFakeSink(1)
	p := &recordingFakeProvider{genOut: ports.Completion{
		Usage: ports.TokenUsage{Input: 10, Output: 20},
	}}
	rec := NewRecording(p, "gemini", "gemini-2.5-flash", sink)

	ctx := ctxkey.WithCompanyID(context.Background(), 7)
	if _, err := rec.Generate(ctx, ports.Prompt{}); err != nil {
		t.Fatalf("Generate err = %v", err)
	}
	sink.wait(t, 1)

	got := sink.get()
	if len(got) != 1 {
		t.Fatalf("records = %d, want 1", len(got))
	}
	l := got[0]
	if l.CompanyID != 7 || l.Vendor != "gemini" || l.Model != "gemini-2.5-flash" || l.Op != "generate" || l.Status != "ok" {
		t.Fatalf("record = %+v, unexpected", l)
	}
	if l.InputTokens != 10 || l.OutputTokens != 20 {
		t.Fatalf("record tokens = %+v, want 10/20", l)
	}
	if l.LatencyMs < 0 {
		t.Fatalf("latency_ms = %d, want >= 0", l.LatencyMs)
	}
}

func TestRecordingGenerateJSONRecordsProviderError(t *testing.T) {
	sink := newFakeSink(1)
	p := &recordingFakeProvider{jsonErr: &llmerr.ProviderError{Vendor: "openai", Status: 429, Message: "rate limited"}}
	rec := NewRecording(p, "openai", "gpt-4o-mini", sink)

	ctx := ctxkey.WithCompanyID(context.Background(), 3)
	if _, err := rec.GenerateJSON(ctx, ports.Prompt{}); err == nil {
		t.Fatal("GenerateJSON err = nil, want error")
	}
	sink.wait(t, 1)

	got := sink.get()
	if len(got) != 1 {
		t.Fatalf("records = %d, want 1", len(got))
	}
	l := got[0]
	if l.Op != "json" || l.Status != "error" || l.HTTPStatus != 429 {
		t.Fatalf("record = %+v, want op=json status=error http_status=429", l)
	}
	if l.Error == "" {
		t.Fatal("record.Error is empty, want message")
	}
}

func TestRecordingStreamRecordsOnceAfterClose(t *testing.T) {
	sink := newFakeSink(1)
	ch := make(chan ports.Completion, 3)
	ch <- ports.Completion{Text: "a"}
	ch <- ports.Completion{Text: "b"}
	ch <- ports.Completion{Vendor: "gemini", Model: "gemini-2.5-flash", Usage: ports.TokenUsage{Input: 5, Output: 15}}
	close(ch)

	p := &recordingFakeProvider{streamCh: ch}
	rec := NewRecording(p, "gemini", "gemini-2.5-flash", sink)

	ctx := ctxkey.WithCompanyID(context.Background(), 9)
	out, err := rec.Stream(ctx, ports.Prompt{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}

	count := 0
	for range out {
		count++
	}
	if count != 3 {
		t.Fatalf("received %d chunks, want 3", count)
	}

	sink.wait(t, 1)
	got := sink.get()
	if len(got) != 1 {
		t.Fatalf("records = %d, want exactly 1", len(got))
	}
	l := got[0]
	if l.Op != "stream" || l.Status != "ok" {
		t.Fatalf("record = %+v, want op=stream status=ok", l)
	}
	if l.InputTokens != 5 || l.OutputTokens != 15 {
		t.Fatalf("record tokens = %+v, want 5/15", l)
	}
}

func TestRecordingStreamRecordsErrorOnInitialFailure(t *testing.T) {
	sink := newFakeSink(1)
	p := &recordingFakeProvider{streamErr: errors.New("boom")}
	rec := NewRecording(p, "gemini", "gemini-2.5-flash", sink)

	ctx := ctxkey.WithCompanyID(context.Background(), 9)
	if _, err := rec.Stream(ctx, ports.Prompt{}); err == nil {
		t.Fatal("Stream err = nil, want error")
	}
	sink.wait(t, 1)

	got := sink.get()
	if len(got) != 1 || got[0].Op != "stream" || got[0].Status != "error" {
		t.Fatalf("record = %+v, want op=stream status=error", got)
	}
}

func TestRecordingSinkDoesNotBlockGenerate(t *testing.T) {
	sink := newFakeSink(1)
	sink.delay = 500 * time.Millisecond
	p := &recordingFakeProvider{genOut: ports.Completion{}}
	rec := NewRecording(p, "gemini", "gemini-2.5-flash", sink)

	ctx := ctxkey.WithCompanyID(context.Background(), 1)
	start := time.Now()
	if _, err := rec.Generate(ctx, ports.Prompt{}); err != nil {
		t.Fatalf("Generate err = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("Generate took %v, want it to return before the sink's 500ms delay", elapsed)
	}
	sink.wait(t, 1)
}
