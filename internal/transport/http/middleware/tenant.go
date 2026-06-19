package middleware

import (
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/shared/ctxkey"
	apperr "github.com/my/app/internal/shared/errors"
	"github.com/my/app/internal/shared/servicejwt"
	"github.com/my/app/internal/transport/http/response"
)

const (
	HeaderTenantID = "X-Tenant-Id"
	headerAuth     = "Authorization"
)

func RequireCompany(jwtSecret string, requireAuth bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()
			headerCID, headerErr := parseTenantHeader(req.Header.Get(HeaderTenantID))

			var cid int64
			var principal ctxkey.Principal
			if jwtSecret != "" {
				token := bearerToken(req.Header.Get(headerAuth))
				if token == "" {
					return response.WriteError(c, apperr.New(apperr.CodeUnauthorized, "service token is required"))
				}
				claims, err := servicejwt.Verify(token, jwtSecret)
				if err != nil {
					return response.WriteError(c, apperr.Wrap(apperr.CodeUnauthorized, "invalid service token", err))
				}
				if headerCID > 0 && headerCID != claims.CID {
					return response.WriteError(c, apperr.New(apperr.CodeForbidden, "X-Tenant-Id does not match service token"))
				}
				cid = claims.CID
				principal = ctxkey.Principal{
					CompanyID: claims.CID,
					UserID:    claims.UID,
					Role:      claims.Role,
					Groups:    claims.Groups,
					Perms:     claims.Perms,
					Coverage:  ctxkey.NormalizeCoverage(claims.Cov, claims.CID),
				}
			} else {
				if requireAuth {
					return response.WriteError(c, apperr.New(apperr.CodeUnauthorized, "service token verification is required but not configured"))
				}
				if headerErr != nil {
					return response.WriteError(c, headerErr)
				}
				cid = headerCID
				principal = ctxkey.Principal{
					CompanyID: cid,
					Role:      "dev",
					Perms:     []string{"*"},
					Coverage:  ctxkey.NormalizeCoverage(nil, cid),
				}
			}

			ctx := ctxkey.WithCompanyID(req.Context(), cid)
			ctx = ctxkey.WithPrincipal(ctx, principal)
			ctx = ctxkey.WithCoverage(ctx, principal.Coverage)
			c.SetRequest(req.WithContext(ctx))
			return next(c)
		}
	}
}

func parseTenantHeader(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, apperr.New(apperr.CodeMissingTenant, "X-Tenant-Id header is required")
	}
	cid, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || cid <= 0 {
		return 0, apperr.New(apperr.CodeInvalidInput, "X-Tenant-Id must be a positive integer")
	}
	return cid, nil
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}
