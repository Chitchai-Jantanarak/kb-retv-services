package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

func TestDeadlineMiddlewareBindsBudgetToContext(t *testing.T) {
	p := BudgetPolicy{Fallback: 22 * time.Second, Headroom: 2 * time.Second, Min: time.Second, Max: 60 * time.Second}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/v1/reply", nil)
	req.Header.Set(HeaderTimeoutMs, "10000")
	c := e.NewContext(req, httptest.NewRecorder())

	var remaining time.Duration
	var hadDeadline bool
	err := Deadline(p)(func(c *echo.Context) error {
		dl, ok := c.Request().Context().Deadline()
		hadDeadline = ok
		remaining = time.Until(dl)
		return nil
	})(c)
	if err != nil {
		t.Fatalf("middleware err = %v", err)
	}
	if !hadDeadline {
		t.Fatal("no deadline bound to request context")
	}
	if remaining < 7*time.Second || remaining > 8*time.Second {
		t.Fatalf("remaining = %v, want ~8s", remaining)
	}
}

func TestBudgetPolicyResolve(t *testing.T) {
	p := BudgetPolicy{
		Fallback: 22 * time.Second,
		Headroom: 2 * time.Second,
		Min:      1 * time.Second,
		Max:      60 * time.Second,
	}
	cases := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"valid header minus headroom", "30000", 28 * time.Second},
		{"missing header uses fallback", "", 20 * time.Second},
		{"oversized clamped to max", "999999999", 58 * time.Second},
		{"tiny floored to min", "500", 1 * time.Second},
		{"garbage uses fallback", "abc", 20 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.Resolve(tc.header); got != tc.want {
				t.Fatalf("Resolve(%q) = %v, want %v", tc.header, got, tc.want)
			}
		})
	}
}
