package llm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/llmerr"
)

type ResilientOptions struct {
	Timeout          time.Duration
	RequestsPerSec   float64
	Burst            int
	BreakerFailures  int
	BreakerCooldown  time.Duration
	MaxRetries       int
	RetryBaseBackoff time.Duration
	RetryMaxBackoff  time.Duration
	Clock            func() time.Time
}

func NewResilient(inner ports.LLMProvider, opts ResilientOptions) ports.LLMProvider {
	if inner == nil {
		return nil
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	w := &resilient{
		inner:      inner,
		timeout:    opts.Timeout,
		maxRetries: opts.MaxRetries,
		retryBase:  opts.RetryBaseBackoff,
		retryMax:   opts.RetryMaxBackoff,
	}
	if opts.RequestsPerSec > 0 {
		burst := opts.Burst
		if burst <= 0 {
			burst = 1
		}
		w.limiter = rate.NewLimiter(rate.Limit(opts.RequestsPerSec), burst)
	}
	if opts.BreakerFailures > 0 && opts.BreakerCooldown > 0 {
		w.breaker = &breaker{
			threshold: opts.BreakerFailures,
			cooldown:  opts.BreakerCooldown,
			clock:     clock,
		}
	}
	return w
}

type resilient struct {
	inner      ports.LLMProvider
	timeout    time.Duration
	maxRetries int
	retryBase  time.Duration
	retryMax   time.Duration
	limiter    *rate.Limiter
	breaker    *breaker
}

var ErrCircuitOpen = errors.New("llm: circuit breaker open")

func (r *resilient) Generate(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	return r.call(ctx, func(ctx context.Context) (ports.Completion, error) {
		return r.inner.Generate(ctx, p)
	})
}

func (r *resilient) GenerateJSON(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	return r.call(ctx, func(ctx context.Context) (ports.Completion, error) {
		return r.inner.GenerateJSON(ctx, p)
	})
}

func (r *resilient) Stream(ctx context.Context, p ports.Prompt) (<-chan ports.Completion, error) {
	if r.breaker != nil && !r.breaker.allow() {
		return nil, ErrCircuitOpen
	}
	if r.limiter != nil {
		if err := r.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("llm: rate limit wait: %w", err)
		}
	}
	return r.inner.Stream(ctx, p)
}

func (r *resilient) call(ctx context.Context, fn func(context.Context) (ports.Completion, error)) (ports.Completion, error) {
	if r.breaker != nil && !r.breaker.allow() {
		return ports.Completion{}, ErrCircuitOpen
	}

	if r.limiter != nil {
		if err := r.limiter.Wait(ctx); err != nil {
			r.recordFailure()
			return ports.Completion{}, fmt.Errorf("llm: rate limit wait: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		out, err := r.attempt(ctx, fn)
		if err == nil {
			r.recordSuccess()
			return out, nil
		}
		lastErr = err

		retryAfter, ok := llmerr.Retryable(err)
		if !ok || attempt == r.maxRetries {
			break
		}
		if werr := r.backoff(ctx, attempt, retryAfter); werr != nil {
			break
		}
	}

	r.recordFailure()
	return ports.Completion{}, lastErr
}

func (r *resilient) attempt(ctx context.Context, fn func(context.Context) (ports.Completion, error)) (ports.Completion, error) {
	if r.timeout <= 0 {
		return fn(ctx)
	}
	callCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	return fn(callCtx)
}

func (r *resilient) backoff(ctx context.Context, attempt int, retryAfter time.Duration) error {
	timer := time.NewTimer(r.backoffDuration(attempt, retryAfter))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *resilient) backoffDuration(attempt int, retryAfter time.Duration) time.Duration {
	base := r.retryBase
	if base <= 0 {
		base = 500 * time.Millisecond
	}
	ceil := r.retryMax
	if ceil <= 0 {
		ceil = 8 * time.Second
	}
	wait := max(time.Duration(float64(base)*math.Pow(2, float64(attempt))), retryAfter)
	return min(wait, ceil)
}

func (r *resilient) recordFailure() {
	if r.breaker != nil {
		r.breaker.recordFailure()
	}
}

func (r *resilient) recordSuccess() {
	if r.breaker != nil {
		r.breaker.recordSuccess()
	}
}

type breaker struct {
	threshold int
	cooldown  time.Duration
	clock     func() time.Time

	mu       sync.Mutex
	failures int
	openedAt time.Time
	state    breakerState
}

type breakerState int

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

func (b *breaker) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == breakerOpen {
		if b.clock().Sub(b.openedAt) >= b.cooldown {
			b.state = breakerHalfOpen
			return true
		}
		return false
	}
	return true
}

func (b *breaker) recordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == breakerHalfOpen {
		b.state = breakerOpen
		b.openedAt = b.clock()
		return
	}
	b.failures++
	if b.failures >= b.threshold {
		b.state = breakerOpen
		b.openedAt = b.clock()
	}
}

func (b *breaker) recordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.state = breakerClosed
}
