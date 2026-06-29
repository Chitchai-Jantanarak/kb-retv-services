package middleware

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

const HeaderTimeoutMs = "X-Timeout-Ms"

type BudgetPolicy struct {
	Fallback time.Duration
	Headroom time.Duration
	Min      time.Duration
	Max      time.Duration
}

func (p BudgetPolicy) Resolve(headerMs string) time.Duration {
	caller := p.Fallback
	if ms, err := strconv.Atoi(strings.TrimSpace(headerMs)); err == nil && ms > 0 {
		caller = time.Duration(ms) * time.Millisecond
	}
	if p.Max > 0 && caller > p.Max {
		caller = p.Max
	}
	return max(caller-p.Headroom, p.Min)
}

func Deadline(p BudgetPolicy) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()
			ctx, cancel := context.WithTimeout(req.Context(), p.Resolve(req.Header.Get(HeaderTimeoutMs)))
			defer cancel()
			c.SetRequest(req.WithContext(ctx))
			return next(c)
		}
	}
}
