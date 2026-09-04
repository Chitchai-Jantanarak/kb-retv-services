package middleware

import (
	"context"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/transport/http/response"
)

type FeatureReader interface {
	Enabled(ctx context.Context, companyID int64, key string) bool
}

func RequireFeature(features FeatureReader, key string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if features == nil {
				return next(c)
			}
			principal, ok := ctxkey.PrincipalFrom(c.Request().Context())
			if !ok {
				return response.WriteError(c, apperr.New(apperr.CodeForbidden, "principal is required"))
			}
			if features.Enabled(c.Request().Context(), principal.CompanyID, key) {
				return next(c)
			}
			return response.WriteError(c, apperr.New(apperr.CodeForbidden, "feature disabled: "+key))
		}
	}
}
