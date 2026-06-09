package middleware

import (
	"fmt"
	"runtime/debug"

	"github.com/labstack/echo/v5"
	"go.uber.org/zap"

	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/shared/logger"
	"github.com/my/app/internal/transport/http/response"
)

func Recover() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					logger.FromContext(c.Request().Context()).Error(
						"panic recovered",
						zap.Any("panic", r),
						zap.ByteString("stack", debug.Stack()),
					)
					err = response.WriteError(c, apperr.Wrap(apperr.CodeInternal, "internal panic", fmt.Errorf("%v", r)))
				}
			}()
			return next(c)
		}
	}
}
