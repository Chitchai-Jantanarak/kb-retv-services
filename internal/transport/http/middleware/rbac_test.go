package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/shared/ctxkey"
)

func TestRequirePermissionAllowsMatchingPerm(t *testing.T) {
	called := false
	err := runRBAC(ctxkey.Principal{CompanyID: 3, Role: "supporter", Perms: []string{"ai:reply:create"}}, "ai:reply:create", func(c *echo.Context) error {
		called = true
		return c.NoContent(http.StatusAccepted)
	})
	if err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestRequirePermissionDeniesRoleWithoutClaim(t *testing.T) {
	for _, role := range []string{"super_admin", "tenant_admin", "agent_lead", "agent", "customer", "unknown_role"} {
		t.Run(role, func(t *testing.T) {
			err := runRBAC(ctxkey.Principal{CompanyID: 3, Role: role}, "ai:review:approve", func(c *echo.Context) error {
				t.Fatal("handler should not be called: role alone must not grant")
				return nil
			})
			if err != nil {
				t.Fatalf("middleware error = %v", err)
			}
		})
	}
}

func TestRequirePermissionWildcardClaimGrants(t *testing.T) {
	called := false
	err := runRBAC(ctxkey.Principal{CompanyID: 3, Role: "super_admin", Perms: []string{"*"}}, "ai:review:approve", func(c *echo.Context) error {
		called = true
		return c.NoContent(http.StatusAccepted)
	})
	if err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestRequirePermissionDeniesMissingPerm(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(ctxkey.WithPrincipal(req.Context(), ctxkey.Principal{CompanyID: 3, Role: "supporter"}))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := RequirePermission("ai:reports:read")(func(c *echo.Context) error {
		t.Fatal("handler should not be called")
		return nil
	})(c)
	if err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequirePermissionDeniesMissingPrincipal(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := RequirePermission("ai:reports:read")(func(c *echo.Context) error {
		t.Fatal("handler should not be called")
		return nil
	})(c)
	if err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRequirePermissionGrantsOnlyByClaim(t *testing.T) {
	cases := []struct {
		role       string
		perms      []string
		permission string
		allow      bool
	}{
		{"agent", []string{"ai:reply:create", "ai:reports:read"}, "ai:reply:create", true},
		{"agent", []string{"ai:reply:create", "ai:reports:read"}, "ai:review:approve", false},
		{"agent_lead", []string{"ai:review:approve"}, "ai:review:approve", true},
		{"customer", []string{"ai:feedback:create"}, "ai:feedback:create", true},
		{"customer", []string{"ai:feedback:create"}, "ai:reply:create", false},
	}
	for _, tc := range cases {
		t.Run(tc.role+"_"+tc.permission, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req = req.WithContext(ctxkey.WithPrincipal(req.Context(), ctxkey.Principal{CompanyID: 3, Role: tc.role, Perms: tc.perms}))
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			called := false
			err := RequirePermission(tc.permission)(func(c *echo.Context) error {
				called = true
				return c.NoContent(http.StatusAccepted)
			})(c)
			if err != nil {
				t.Fatalf("middleware error = %v", err)
			}
			if called != tc.allow {
				t.Fatalf("called = %v, want %v (status %d)", called, tc.allow, rec.Code)
			}
		})
	}
}

func runRBAC(principal ctxkey.Principal, permission string, next echo.HandlerFunc) error {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := ctxkey.WithPrincipal(context.Background(), principal)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return RequirePermission(permission)(next)(c)
}
