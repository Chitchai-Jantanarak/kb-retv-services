package budget

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/my/app/internal/domain/ports"
)

type Config struct {
	Cache                    ports.Cache
	MaxCallsPerRequest       int
	MaxCallsPerCompanyWindow int
	Window                   time.Duration
}

type Limiter struct {
	cache        ports.Cache
	maxPerReq    int
	maxPerWindow int
	window       time.Duration
}

func New(cfg Config) *Limiter {
	window := cfg.Window
	if window <= 0 {
		window = time.Minute
	}
	return &Limiter{
		cache:        cfg.Cache,
		maxPerReq:    cfg.MaxCallsPerRequest,
		maxPerWindow: cfg.MaxCallsPerCompanyWindow,
		window:       window,
	}
}

type requestKey struct{}

func (l *Limiter) Begin(ctx context.Context) context.Context {
	return context.WithValue(ctx, requestKey{}, new(atomic.Int32))
}

func (l *Limiter) Acquire(ctx context.Context, companyID int64, _ string) (bool, error) {
	if !l.allowRequest(ctx) {
		return false, nil
	}
	return l.allowWindow(ctx, companyID)
}

func (l *Limiter) allowRequest(ctx context.Context) bool {
	if l.maxPerReq <= 0 {
		return true
	}
	counter, ok := ctx.Value(requestKey{}).(*atomic.Int32)
	if !ok {
		return true
	}
	return int(counter.Add(1)) <= l.maxPerReq
}

func (l *Limiter) allowWindow(ctx context.Context, companyID int64) (bool, error) {
	if l.maxPerWindow <= 0 || l.cache == nil {
		return true, nil
	}
	bucket := time.Now().Unix() / int64(l.window.Seconds())
	key := fmt.Sprintf("rag:budget:%d:%d", companyID, bucket)

	raw, err := l.cache.Get(ctx, key)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		raw = nil
	}
	used := 0
	if len(raw) > 0 {
		if n, perr := strconv.Atoi(string(raw)); perr == nil {
			used = n
		}
	}
	if used >= l.maxPerWindow {
		return false, nil
	}
	next := []byte(strconv.Itoa(used + 1))
	if err := l.cache.Set(ctx, key, next, l.window); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
	}
	return true, nil
}
