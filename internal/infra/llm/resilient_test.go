package llm

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/my/app/internal/domain/ports"
)

type stubProvider struct {
	calls atomic.Int64
	err   error
	delay time.Duration
}

func (s *stubProvider) Generate(ctx context.Context, _ ports.Prompt) (ports.Completion, error) {
	s.calls.Add(1)
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return ports.Completion{}, ctx.Err()
		}
	}
	if s.err != nil {
		return ports.Completion{}, s.err
	}
	return ports.Completion{Text: "ok"}, nil
}

func (s *stubProvider) GenerateJSON(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	return s.Generate(ctx, p)
}

func (s *stubProvider) Stream(_ context.Context, _ ports.Prompt) (<-chan ports.Completion, error) {
	return nil, errors.New("stub: no stream")
}

func TestResilientNilInnerReturnsNil(t *testing.T) {
	if NewResilient(nil, ResilientOptions{}) != nil {
		t.Fatal("NewResilient(nil, ...) should return nil")
	}
}

func TestResilientTimeoutCancelsSlowCall(t *testing.T) {
	inner := &stubProvider{delay: 100 * time.Millisecond}
	r := NewResilient(inner, ResilientOptions{Timeout: 10 * time.Millisecond})
	_, err := r.Generate(context.Background(), ports.Prompt{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
}

func TestResilientCircuitBreakerOpensAfterThreshold(t *testing.T) {
	clock := time.Unix(0, 0)
	tickClock := func() time.Time { return clock }

	inner := &stubProvider{err: errors.New("upstream fail")}
	r := NewResilient(inner, ResilientOptions{
		BreakerFailures: 3,
		BreakerCooldown: time.Second,
		Clock:           tickClock,
	})

	for i := range 3 {
		if _, err := r.Generate(context.Background(), ports.Prompt{}); err == nil {
			t.Fatalf("iter %d: err = nil, want upstream fail", i)
		}
	}

	if _, err := r.Generate(context.Background(), ports.Prompt{}); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen", err)
	}
	if inner.calls.Load() != 3 {
		t.Fatalf("inner.calls = %d, want 3 (4th must short-circuit)", inner.calls.Load())
	}
}

func TestResilientCircuitBreakerHalfOpenRecovers(t *testing.T) {
	clock := time.Unix(0, 0)
	clockFn := func() time.Time { return clock }

	inner := &stubProvider{err: errors.New("fail")}
	r := NewResilient(inner, ResilientOptions{
		BreakerFailures: 1,
		BreakerCooldown: time.Second,
		Clock:           clockFn,
	})

	// Trip the breaker
	_, _ = r.Generate(context.Background(), ports.Prompt{})
	if _, err := r.Generate(context.Background(), ports.Prompt{}); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected open after 1 failure, got %v", err)
	}

	// Advance clock past cooldown; flip inner to succeed
	clock = clock.Add(2 * time.Second)
	inner.err = nil
	if _, err := r.Generate(context.Background(), ports.Prompt{}); err != nil {
		t.Fatalf("half-open success path err = %v", err)
	}

	// Closed again — must accept normal traffic
	if _, err := r.Generate(context.Background(), ports.Prompt{}); err != nil {
		t.Fatalf("post-recovery err = %v", err)
	}
}

func TestResilientHalfOpenFailureReopens(t *testing.T) {
	clock := time.Unix(0, 0)
	clockFn := func() time.Time { return clock }

	inner := &stubProvider{err: errors.New("fail")}
	r := NewResilient(inner, ResilientOptions{
		BreakerFailures: 1,
		BreakerCooldown: time.Second,
		Clock:           clockFn,
	})

	_, _ = r.Generate(context.Background(), ports.Prompt{})

	clock = clock.Add(2 * time.Second)
	// half-open probe still fails — breaker must re-open immediately
	_, _ = r.Generate(context.Background(), ports.Prompt{})

	// Next call within cooldown must short-circuit
	if _, err := r.Generate(context.Background(), ports.Prompt{}); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen after failed probe", err)
	}
}

func TestResilientRateLimitWaitFailsOnCancelledContext(t *testing.T) {
	inner := &stubProvider{}
	r := NewResilient(inner, ResilientOptions{RequestsPerSec: 0.0001, Burst: 1})

	// Burn the burst
	if _, err := r.Generate(context.Background(), ports.Prompt{}); err != nil {
		t.Fatalf("first call err = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.Generate(ctx, ports.Prompt{})
	if err == nil || !strings.Contains(err.Error(), "rate limit wait") {
		t.Fatalf("err = %v, want rate-limit error", err)
	}
}
