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

func TestRequirePermissionAllowsSuperAdmin(t *testing.T) {
	called := false
	err := runRBAC(ctxkey.Principal{CompanyID: 3, Role: "super_admin"}, "ai:review:approve", func(c *echo.Context) error {
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

func runRBAC(principal ctxkey.Principal, permission string, next echo.HandlerFunc) error {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := ctxkey.WithPrincipal(context.Background(), principal)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return RequirePermission(permission)(next)(c)
}
