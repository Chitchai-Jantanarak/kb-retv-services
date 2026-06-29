package llm

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/llmerr"
)

type flakyProvider struct {
	calls   atomic.Int64
	failN   int64
	failErr error
}

func (f *flakyProvider) Generate(_ context.Context, _ ports.Prompt) (ports.Completion, error) {
	if f.calls.Add(1) <= f.failN {
		return ports.Completion{}, f.failErr
	}
	return ports.Completion{Text: "ok"}, nil
}

func (f *flakyProvider) GenerateJSON(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	return f.Generate(ctx, p)
}

func (f *flakyProvider) Stream(_ context.Context, _ ports.Prompt) (<-chan ports.Completion, error) {
	return nil, errors.New("flaky: no stream")
}

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

func TestResilientRetriesRetryableErrorThenSucceeds(t *testing.T) {
	inner := &flakyProvider{failN: 1, failErr: &llmerr.ProviderError{Vendor: "gemini", Status: 503}}
	r := NewResilient(inner, ResilientOptions{
		MaxRetries:       2,
		RetryBaseBackoff: time.Millisecond,
		RetryMaxBackoff:  time.Millisecond,
	})

	out, err := r.Generate(context.Background(), ports.Prompt{})
	if err != nil {
		t.Fatalf("Generate err = %v, want nil after one retry", err)
	}
	if out.Text != "ok" {
		t.Fatalf("out.Text = %q, want ok", out.Text)
	}
	if got := inner.calls.Load(); got != 2 {
		t.Fatalf("inner.calls = %d, want 2 (1 fail + 1 retry success)", got)
	}
}

func TestResilientDoesNotRetryNonRetryableError(t *testing.T) {
	inner := &flakyProvider{failN: 100, failErr: &llmerr.ProviderError{Vendor: "gemini", Status: 400}}
	r := NewResilient(inner, ResilientOptions{MaxRetries: 3, RetryBaseBackoff: time.Millisecond})

	_, err := r.Generate(context.Background(), ports.Prompt{})
	if err == nil {
		t.Fatal("want error for status 400")
	}
	if got := inner.calls.Load(); got != 1 {
		t.Fatalf("inner.calls = %d, want 1 (4xx must not retry)", got)
	}
}

func TestResilientGivesUpAfterMaxRetries(t *testing.T) {
	wantErr := &llmerr.ProviderError{Vendor: "gemini", Status: 503}
	inner := &flakyProvider{failN: 100, failErr: wantErr}
	r := NewResilient(inner, ResilientOptions{MaxRetries: 2, RetryBaseBackoff: time.Millisecond, RetryMaxBackoff: time.Millisecond})

	_, err := r.Generate(context.Background(), ports.Prompt{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want the provider error", err)
	}
	if got := inner.calls.Load(); got != 3 {
		t.Fatalf("inner.calls = %d, want 3 (1 + 2 retries)", got)
	}
}

func TestResilientRetryStopsOnCanceledContext(t *testing.T) {
	inner := &flakyProvider{failN: 100, failErr: &llmerr.ProviderError{Vendor: "gemini", Status: 503}}
	r := NewResilient(inner, ResilientOptions{MaxRetries: 5, RetryBaseBackoff: 50 * time.Millisecond, RetryMaxBackoff: time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, err := r.Generate(ctx, ports.Prompt{})
	if elapsed := time.Since(start); elapsed > 25*time.Millisecond {
		t.Fatalf("elapsed %v: slept backoff instead of failing fast on canceled ctx", elapsed)
	}
	if err == nil {
		t.Fatal("want error")
	}
	if got := inner.calls.Load(); got != 1 {
		t.Fatalf("inner.calls = %d, want 1 (out-of-time must stop retrying)", got)
	}
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

	_, _ = r.Generate(context.Background(), ports.Prompt{})
	if _, err := r.Generate(context.Background(), ports.Prompt{}); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected open after 1 failure, got %v", err)
	}

	clock = clock.Add(2 * time.Second)
	inner.err = nil
	if _, err := r.Generate(context.Background(), ports.Prompt{}); err != nil {
		t.Fatalf("half-open success path err = %v", err)
	}

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
	_, _ = r.Generate(context.Background(), ports.Prompt{})

	if _, err := r.Generate(context.Background(), ports.Prompt{}); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("err = %v, want ErrCircuitOpen after failed probe", err)
	}
}

func TestResilientRateLimitWaitFailsOnCancelledContext(t *testing.T) {
	inner := &stubProvider{}
	r := NewResilient(inner, ResilientOptions{RequestsPerSec: 0.0001, Burst: 1})

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
