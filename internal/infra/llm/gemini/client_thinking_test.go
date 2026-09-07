package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/my/app/internal/domain/ports"
)

func intPtr(v int) *int { return &v }

func buildRequestGenConfig(t *testing.T, model string, thinkBudget *int, maxToks int) map[string]any {
	t.Helper()
	c, err := New(Config{APIKey: "k", Model: model})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	req := c.buildRequest(ports.Prompt{User: "x", ThinkBudget: thinkBudget, MaxToks: maxToks}, "")
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal() err = %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("Unmarshal() err = %v", err)
	}
	genCfg, _ := m["generationConfig"].(map[string]any)
	return genCfg
}

func TestBuildRequestThinkingBudgetZeroOnGemini25(t *testing.T) {
	genCfg := buildRequestGenConfig(t, "gemini-2.5-flash", intPtr(0), 0)
	if genCfg == nil {
		t.Fatal("generationConfig = nil, want thinkingConfig present")
	}
	thinkCfg, ok := genCfg["thinkingConfig"].(map[string]any)
	if !ok {
		t.Fatalf("thinkingConfig missing: %+v", genCfg)
	}
	budget, ok := thinkCfg["thinkingBudget"]
	if !ok {
		t.Fatalf("thinkingBudget missing: %+v", thinkCfg)
	}
	if budget != float64(0) {
		t.Fatalf("thinkingBudget = %v, want 0", budget)
	}
}

func TestBuildRequestThinkingBudgetZeroOnGemini35OmitsThinkingConfig(t *testing.T) {
	genCfg := buildRequestGenConfig(t, "gemini-3.5-flash-lite", intPtr(0), 0)
	if genCfg == nil {
		return
	}
	if _, ok := genCfg["thinkingConfig"]; ok {
		t.Fatalf("thinkingConfig = %+v, want absent for gemini-3.5 with ThinkBudget=0", genCfg["thinkingConfig"])
	}
}

func TestBuildRequestThinkingBudgetNonZeroOnGemini35PassesThrough(t *testing.T) {
	genCfg := buildRequestGenConfig(t, "gemini-3.5-flash-lite", intPtr(512), 0)
	if genCfg == nil {
		t.Fatal("generationConfig = nil, want thinkingConfig present")
	}
	thinkCfg, ok := genCfg["thinkingConfig"].(map[string]any)
	if !ok {
		t.Fatalf("thinkingConfig missing: %+v", genCfg)
	}
	if thinkCfg["thinkingBudget"] != float64(512) {
		t.Fatalf("thinkingBudget = %v, want 512", thinkCfg["thinkingBudget"])
	}
}

func TestBuildRequestNilThinkBudgetOnGemini35OmitsThinkingConfigButKeepsMaxTokens(t *testing.T) {
	genCfg := buildRequestGenConfig(t, "gemini-3.5-flash-lite", nil, 8)
	if genCfg == nil {
		t.Fatal("generationConfig = nil, want maxOutputTokens present")
	}
	if _, ok := genCfg["thinkingConfig"]; ok {
		t.Fatalf("thinkingConfig = %+v, want absent when ThinkBudget is nil", genCfg["thinkingConfig"])
	}
	if genCfg["maxOutputTokens"] != float64(8) {
		t.Fatalf("maxOutputTokens = %v, want 8", genCfg["maxOutputTokens"])
	}
}

func TestGenerateUsageIncludesThoughtsTokensInOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":42,"candidatesTokenCount":18,"thoughtsTokenCount":305,"totalTokenCount":365}}`))
	}))
	defer srv.Close()

	c, err := New(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	out, err := c.Generate(context.Background(), ports.Prompt{User: "x"})
	if err != nil {
		t.Fatalf("Generate() err = %v", err)
	}
	if out.Usage.Input != 42 {
		t.Fatalf("Input = %d, want 42", out.Usage.Input)
	}
	if out.Usage.Output != 323 {
		t.Fatalf("Output = %d, want 323 (18 candidates + 305 thoughts)", out.Usage.Output)
	}
	if out.Usage.Total != 365 {
		t.Fatalf("Total = %d, want 365", out.Usage.Total)
	}
}
