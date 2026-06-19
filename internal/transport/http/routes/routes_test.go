package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"go.uber.org/zap"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/workflows/omnichannel"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/transport/http/handlers"
)

func TestRegisterInstallsHealthz(t *testing.T) {
	e := echo.New()
	Register(e, handlers.NewReplyHandler(fakeWorkflow{}), Options{Log: zap.NewNop()})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReplyTenantHeaderFailureCases(t *testing.T) {
	cases := []struct {
		name       string
		setHeader  bool
		headerVal  string
		wantStatus int
	}{
		{"missing header", false, "", http.StatusUnauthorized},
		{"empty header", true, "", http.StatusUnauthorized},
		{"whitespace only", true, "   ", http.StatusUnauthorized},
		{"non-numeric", true, "abc", http.StatusBadRequest},
		{"zero value", true, "0", http.StatusBadRequest},
		{"negative value", true, "-1", http.StatusBadRequest},
		{"overflow int64", true, "99999999999999999999", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			Register(e, handlers.NewReplyHandler(fakeWorkflow{}), Options{Log: zap.NewNop()})

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/reply", strings.NewReader(`{"customer_id":"c1","message":"offline"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			if tc.setHeader {
				req.Header.Set("X-Tenant-Id", tc.headerVal)
			}
			e.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestReplyUsesTenantHeader(t *testing.T) {
	e := echo.New()
	Register(e, handlers.NewReplyHandler(fakeWorkflow{}), Options{Log: zap.NewNop()})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/reply", strings.NewReader(`{"customer_id":"c1","message":"offline"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("X-Tenant-Id", "3")
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		Success bool              `json:"success"`
		Data    dto.ReplyResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if body.Data.Intent != "ok" {
		t.Fatalf("intent = %q, want ok", body.Data.Intent)
	}
}

func TestRegisterUsesReplyRoutes(t *testing.T) {
	e := echo.New()
	Register(e, handlers.NewReplyHandler(fakeWorkflow{}), Options{
		Log:      zap.NewNop(),
		Feedback: handlers.NewFeedbackHandler(stubFeedbackRecorder{}),
	})

	want := map[string]bool{
		http.MethodPost + " /v1/reply":          false,
		http.MethodPost + " /v1/reply/feedback": false,
	}
	for _, route := range e.Router().Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
		if strings.Contains(route.Path, "smart-reply") {
			t.Fatalf("legacy smart-reply route still registered: %s %s", route.Method, route.Path)
		}
	}
	for route, found := range want {
		if !found {
			t.Fatalf("route %s not registered", route)
		}
	}
}

func TestInboundRouteDoesNotRequireTenantAuth(t *testing.T) {
	registry, err := omnichannel.NewNormalizerRegistry()
	if err != nil {
		t.Fatalf("NewNormalizerRegistry() error = %v", err)
	}
	e := echo.New()
	Register(e, handlers.NewReplyHandler(fakeWorkflow{}), Options{
		Log:     zap.NewNop(),
		Inbound: handlers.NewInboundHandler(fakeInboundWorkflow{}, registry),
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/inbound/line", strings.NewReader(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	e.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("inbound route should not require tenant auth, got status %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d from unknown channel validation", rec.Code, http.StatusBadRequest)
	}
}

func TestRegisterSkipsSwaggerWhenDisabled(t *testing.T) {
	e := echo.New()
	Register(e, handlers.NewReplyHandler(fakeWorkflow{}), Options{Log: zap.NewNop()})

	for _, path := range []string{"/swagger/index.html", "/swagger/doc.json"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
			}
		})
	}
}

func TestRegisterInstallsSwaggerWhenEnabled(t *testing.T) {
	e := echo.New()
	Register(e, handlers.NewReplyHandler(fakeWorkflow{}), Options{Log: zap.NewNop(), SwaggerEnabled: true})

	found := false
	for _, route := range e.Router().Routes() {
		if route.Method == http.MethodGet && route.Path == "/swagger/*" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("swagger route not registered")
	}
}

func TestRegisterServesSwaggerDocWhenEnabled(t *testing.T) {
	e := echo.New()
	Register(e, handlers.NewReplyHandler(fakeWorkflow{}), Options{Log: zap.NewNop(), SwaggerEnabled: true})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/swagger/doc.json", nil)
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body struct {
		Swagger string `json:"swagger"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if body.Swagger != "2.0" {
		t.Fatalf("swagger = %q, want 2.0", body.Swagger)
	}
}

func TestRegisterServesScalarDocsWhenEnabled(t *testing.T) {
	e := echo.New()
	Register(e, handlers.NewReplyHandler(fakeWorkflow{}), Options{Log: zap.NewNop(), SwaggerEnabled: true})

	openapi := httptest.NewRecorder()
	e.ServeHTTP(openapi, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if openapi.Code != http.StatusOK {
		t.Fatalf("/openapi.json status = %d, want %d", openapi.Code, http.StatusOK)
	}
	var spec struct {
		Swagger string `json:"swagger"`
	}
	if err := json.Unmarshal(openapi.Body.Bytes(), &spec); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if spec.Swagger != "2.0" {
		t.Fatalf("swagger = %q, want 2.0", spec.Swagger)
	}

	docs := httptest.NewRecorder()
	e.ServeHTTP(docs, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if docs.Code != http.StatusOK {
		t.Fatalf("/docs status = %d, want %d", docs.Code, http.StatusOK)
	}
	if !strings.Contains(docs.Body.String(), "Scalar.createApiReference") {
		t.Fatalf("/docs body missing Scalar bootstrap")
	}

	redirect := httptest.NewRecorder()
	e.ServeHTTP(redirect, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if redirect.Code != http.StatusTemporaryRedirect {
		t.Fatalf("/docs/ status = %d, want %d", redirect.Code, http.StatusTemporaryRedirect)
	}
}

func TestRegisterSkipsScalarDocsWhenDisabled(t *testing.T) {
	e := echo.New()
	Register(e, handlers.NewReplyHandler(fakeWorkflow{}), Options{Log: zap.NewNop()})

	for _, path := range []string{"/openapi.json", "/docs"} {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

type fakeWorkflow struct{}

func (fakeWorkflow) Run(ctx context.Context, req dto.ReplyRequest) (dto.ReplyResponse, error) {
	return dto.ReplyResponse{Intent: "ok"}, nil
}

type fakeInboundWorkflow struct{}

func (fakeInboundWorkflow) Run(context.Context, omnichannel.Normalized, []byte) (omnichannel.Result, error) {
	return omnichannel.Result{}, nil
}

type stubFeedbackRecorder struct{}

func (stubFeedbackRecorder) RecordFeedback(context.Context, ports.AIActionFeedback) error {
	return nil
}
