package llm

import (
	"context"
	"testing"
	"time"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

type scopeSink struct {
	got     chan int64
	sawZero chan bool
}

func (s *scopeSink) Record(ctx context.Context, l RequestLog) error {
	cid, ok := ctxkey.CompanyID(ctx)
	if !ok {
		s.sawZero <- true
		return nil
	}
	s.got <- cid
	return nil
}

type okProvider struct{}

func (okProvider) Generate(context.Context, ports.Prompt) (ports.Completion, error) {
	return ports.Completion{Text: "ok"}, nil
}
func (okProvider) GenerateJSON(context.Context, ports.Prompt) (ports.Completion, error) {
	return ports.Completion{Text: "{}"}, nil
}
func (okProvider) Stream(context.Context, ports.Prompt) (<-chan ports.Completion, error) {
	ch := make(chan ports.Completion)
	close(ch)
	return ch, nil
}

// The sink writes through the tenant router, which picks the tenant database
// from the company id on its context. Detaching to context.Background() drops
// that scope and every insert is refused.
func TestRecordingSinkContextCarriesCompanyScope(t *testing.T) {
	sink := &scopeSink{got: make(chan int64, 1), sawZero: make(chan bool, 1)}
	p := NewRecording(okProvider{}, "gemini", "m", sink)

	ctx := ctxkey.WithCompanyID(context.Background(), 42)
	if _, err := p.Generate(ctx, ports.Prompt{}); err != nil {
		t.Fatalf("generate: %v", err)
	}

	select {
	case cid := <-sink.got:
		if cid != 42 {
			t.Fatalf("sink ctx company_id = %d, want 42", cid)
		}
	case <-sink.sawZero:
		t.Fatal("sink context has no company_id; the tenant router cannot route the insert")
	case <-time.After(2 * time.Second):
		t.Fatal("sink never called")
	}
}

func TestRecordingSinkSurvivesCancelledRequestContext(t *testing.T) {
	sink := &scopeSink{got: make(chan int64, 1), sawZero: make(chan bool, 1)}
	p := NewRecording(okProvider{}, "gemini", "m", sink)

	reqCtx, cancel := context.WithCancel(ctxkey.WithCompanyID(context.Background(), 7))
	if _, err := p.Generate(reqCtx, ports.Prompt{}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	cancel()

	select {
	case cid := <-sink.got:
		if cid != 7 {
			t.Fatalf("sink ctx company_id = %d, want 7", cid)
		}
	case <-sink.sawZero:
		t.Fatal("sink context has no company_id")
	case <-time.After(2 * time.Second):
		t.Fatal("sink never called after request cancellation")
	}
}
