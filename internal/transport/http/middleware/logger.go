package middleware

import (
	"time"

	"github.com/labstack/echo/v5"
	"go.uber.org/zap"

	"github.com/my/app/internal/shared/ctxkey"
	"github.com/my/app/internal/shared/logger"
)

func Logger(base *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			start := time.Now()
			req := c.Request()

			cid, _ := ctxkey.CompanyID(req.Context())
			bound := base.With(
				zap.String("request_id", ctxkey.RequestID(req.Context())),
				zap.Int64("company_id", cid),
				zap.String("method", req.Method),
				zap.String("path", req.URL.Path),
			)
			ctx := logger.WithContext(req.Context(), bound)
			c.SetRequest(req.WithContext(ctx))

			err := next(c)

			bound.Info(
				"http_request",
				zap.Int("status", responseStatus(c)),
				zap.Duration("latency", time.Since(start)),
				zap.Error(err),
			)
			return err
		}
	}
}

func responseStatus(c *echo.Context) int {
	resp, err := echo.UnwrapResponse(c.Response())
	if err != nil {
		return 0
	}
	return resp.Status
}
