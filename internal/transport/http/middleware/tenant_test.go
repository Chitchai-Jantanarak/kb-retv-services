package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/shared/ctxkey"
)

func TestRequireCompanyProdEmptySecretRejects(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/reply", nil)
	req.Header.Set(HeaderTenantID, "3")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := RequireCompany("", true)(func(c *echo.Context) error {
		t.Fatal("handler must not run when auth is required but unconfigured")
		return nil
	})(c)
	if err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireCompanyDevEmptySecretTrustsHeader(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/reply", nil)
	req.Header.Set(HeaderTenantID, "3")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var got ctxkey.Principal
	called := false
	err := RequireCompany("", false)(func(c *echo.Context) error {
		called = true
		p, _ := ctxkey.PrincipalFrom(c.Request().Context())
		got = p
		return c.NoContent(http.StatusAccepted)
	})(c)
	if err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if !called {
		t.Fatal("handler was not called in dev mode")
	}
	if got.CompanyID != 3 || got.Role != "dev" {
		t.Fatalf("principal = %+v, want company 3 role dev", got)
	}
}

func TestRequireCompanyDevEmptySecretMissingHeaderRejects(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/reply", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := RequireCompany("", false)(func(c *echo.Context) error {
		t.Fatal("handler must not run without a tenant header")
		return nil
	})(c)
	if err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
