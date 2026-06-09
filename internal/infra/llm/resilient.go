package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/my/app/internal/domain/ports"
)

type ResilientOptions struct {
	Timeout         time.Duration
	RequestsPerSec  float64
	Burst           int
	BreakerFailures int
	BreakerCooldown time.Duration
	Clock           func() time.Time
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
		inner:   inner,
		timeout: opts.Timeout,
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
	inner   ports.LLMProvider
	timeout time.Duration
	limiter *rate.Limiter
	breaker *breaker
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

	callCtx := ctx
	if r.timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}

	out, err := fn(callCtx)
	if err != nil {
		r.recordFailure()
		return ports.Completion{}, err
	}
	r.recordSuccess()
	return out, nil
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
