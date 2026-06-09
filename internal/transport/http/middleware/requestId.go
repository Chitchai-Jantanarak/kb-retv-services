package middleware

import (
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/shared/ctxkey"
)

const HeaderRequestID = "X-Request-Id"

func RequestID() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			id := c.Request().Header.Get(HeaderRequestID)
			if id == "" {
				id = uuid.NewString()
			}
			c.Response().Header().Set(HeaderRequestID, id)

			req := c.Request()
			ctx := ctxkey.WithRequestID(req.Context(), id)
			c.SetRequest(req.WithContext(ctx))
			return next(c)
		}
	}
}
