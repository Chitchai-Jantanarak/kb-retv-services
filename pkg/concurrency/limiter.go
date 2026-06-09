package concurrency

import "context"

type Limiter struct {
	sem chan struct{}
}

func NewLimiter(maxConcurrent int) *Limiter {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	return &Limiter{sem: make(chan struct{}, maxConcurrent)}
}

func (l *Limiter) Acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case l.sem <- struct{}{}:
		return nil
	}
}

func (l *Limiter) Release() {
	select {
	case <-l.sem:
	default:
	}
}

func (l *Limiter) Do(ctx context.Context, fn func() error) error {
	if err := l.Acquire(ctx); err != nil {
		return err
	}
	defer l.Release()
	return fn()
}
