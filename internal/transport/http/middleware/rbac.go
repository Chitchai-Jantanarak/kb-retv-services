package middleware

import (
	"slices"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/transport/http/response"
)

func RequirePermission(permission string) echo.MiddlewareFunc {
	required := strings.TrimSpace(permission)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			principal, ok := ctxkey.PrincipalFrom(c.Request().Context())
			if !ok {
				return response.WriteError(c, apperr.New(apperr.CodeForbidden, "principal is required"))
			}
			if can(principal, required) {
				return next(c)
			}
			return response.WriteError(c, apperr.New(apperr.CodeForbidden, "permission denied"))
		}
	}
}

func can(principal ctxkey.Principal, permission string) bool {
	if permission == "" {
		return true
	}
	if principal.Role == "super_admin" {
		return true
	}
	return slices.Contains(principal.Perms, "*") || slices.Contains(principal.Perms, permission)
}
