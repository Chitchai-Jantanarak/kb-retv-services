package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/shared/ctxkey"
)

type fakeFeatures struct{ off map[string]bool }

func (f fakeFeatures) Enabled(_ context.Context, _ int64, key string) bool { return !f.off[key] }

func TestRequireFeatureGatesWith403(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", nil)
	req = req.WithContext(ctxkey.WithPrincipal(ctxkey.WithCompanyID(req.Context(), 3), ctxkey.Principal{CompanyID: 3}))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	called := false
	err := RequireFeature(fakeFeatures{off: map[string]bool{"feature.ai.enabled": true}}, "feature.ai.enabled")(func(c *echo.Context) error {
		called = true
		return nil
	})(c)
	if err != nil {
		t.Fatalf("middleware error = %v", err)
	}
	if called || rec.Code != http.StatusForbidden {
		t.Fatalf("called=%v status=%d, want not called + 403", called, rec.Code)
	}
}

func TestRequireFeaturePassesWhenEnabled(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat", nil)
	req = req.WithContext(ctxkey.WithPrincipal(ctxkey.WithCompanyID(req.Context(), 3), ctxkey.Principal{CompanyID: 3}))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	called := false
	_ = RequireFeature(fakeFeatures{}, "feature.ai.enabled")(func(c *echo.Context) error { called = true; return nil })(c)
	if !called {
		t.Fatal("handler must run when the feature is enabled")
	}
}
