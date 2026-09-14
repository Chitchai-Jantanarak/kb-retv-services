package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/my/app/internal/infra/llm"
	"github.com/my/app/internal/shared/ctxkey"
	appmiddleware "github.com/my/app/internal/transport/http/middleware"
)

type chatModelReaderStub struct {
	companyID int64
	calls     int
}

func (s *chatModelReaderStub) TaskModels(_ context.Context, companyID int64, task string) (llm.TaskModels, error) {
	s.companyID = companyID
	s.calls++
	return llm.TaskModels{Models: []llm.TaskModel{{ConnectionID: 12, Provider: "openai", Model: task + "-model"}}}, nil
}

func TestChatModelsHandlerUsesCallerCompanyAndPermission(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		e := echo.New()
		reader := &chatModelReaderStub{}
		e.GET("/chat/models", NewChatModelsHandler(reader).List, appmiddleware.RequirePermission("ai:reply:create"))
		ctx := ctxkey.WithCompanyID(context.Background(), 7)
		principal := ctxkey.Principal{}
		if allowed {
			principal.Perms = []string{"ai:reply:create"}
		}
		ctx = ctxkey.WithPrincipal(ctx, principal)
		req := httptest.NewRequest(http.MethodGet, "/chat/models?company_id=8", nil).WithContext(ctx)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if allowed && (rec.Code != 200 || reader.companyID != 7 || !strings.Contains(rec.Body.String(), "chat-model")) {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if !allowed && (rec.Code != 403 || reader.calls != 0) {
			t.Fatal("unauthorized caller reached model catalog")
		}
	}
}

func TestModelCatalogHandlerRequiresCompanyContext(t *testing.T) {
	e := echo.New()
	handler := NewAIModelsHandler(nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/models", nil), rec)
	if err := handler.List(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rec.Code)
	}
}
