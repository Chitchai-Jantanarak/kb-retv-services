package handlers

import (
	"context"
	"encoding/json"
	"testing"

	aimysql "github.com/my/app/internal/repositories/ai/mysql"
)

type stubAIUsage struct {
	snap    aimysql.UsageSnapshot
	err     error
	lastCID int64
	lastDay int
}

func (s *stubAIUsage) Usage(_ context.Context, companyID int64, days int) (aimysql.UsageSnapshot, error) {
	s.lastCID = companyID
	s.lastDay = days
	return s.snap, s.err
}

func TestAIUsageHandlerDefaultsAndClampsDays(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  int
	}{
		{name: "no_param_defaults_7", query: "", want: 7},
		{name: "zero_defaults_7", query: "?days=0", want: 7},
		{name: "over_30_clamps_30", query: "?days=99", want: 30},
		{name: "within_range_passes_through", query: "?days=14", want: 14},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &stubAIUsage{}
			h := NewAIUsageHandler(stub)
			c, rec := newRequestWithCompany("GET", "/v1/ai/usage"+tc.query)
			if err := h.Usage(c); err != nil {
				t.Fatalf("Usage: %v", err)
			}
			if rec.Code != 200 {
				t.Fatalf("status = %d", rec.Code)
			}
			if stub.lastDay != tc.want {
				t.Fatalf("days passed to repo = %d, want %d", stub.lastDay, tc.want)
			}
			if stub.lastCID != 7 {
				t.Fatalf("company_id = %d, want 7", stub.lastCID)
			}
		})
	}
}

func TestAIUsageHandlerReturnsShape(t *testing.T) {
	stub := &stubAIUsage{snap: aimysql.UsageSnapshot{
		Totals: aimysql.UsageTotals{Requests: 3, Errors: 1, InputTokens: 100, OutputTokens: 50, LatencyAvgMs: 812, LatencyP50Ms: 700, LatencyP95Ms: 950},
		Today:  aimysql.UsageToday{Requests: 1, Errors: 0, InputTokens: 40, OutputTokens: 20},
		Daily: []aimysql.UsageDay{
			{Date: "2026-09-04", Requests: 3, ByVendor: map[string]int{"gemini": 3}, ByStatus: map[string]int{"429": 1}},
		},
		ByModel: []aimysql.UsageModel{
			{Vendor: "gemini", Model: "gemini-2.5-flash", Requests: 3, Errors: 1, InputTokens: 100, OutputTokens: 50, LatencyAvgMs: 812},
		},
		ModelsDaily: []aimysql.UsageModelDaily{
			{Vendor: "gemini", Model: "gemini-2.5-flash", Daily: []aimysql.UsageModelDay{{Date: "2026-09-04", Requests: 3}}},
		},
		Recent: []aimysql.UsageRow{
			{Timestamp: "2026-09-04T12:00:00Z", Vendor: "gemini", Model: "gemini-2.5-flash", Op: "generate", Status: "ok", HTTPStatus: 200, LatencyMs: 812, InputTokens: 1200, OutputTokens: 300},
		},
	}}
	h := NewAIUsageHandler(stub)
	c, rec := newRequestWithCompany("GET", "/v1/ai/usage?days=7")
	if err := h.Usage(c); err != nil {
		t.Fatalf("Usage: %v", err)
	}

	var payload struct {
		Data struct {
			Days        int              `json:"days"`
			Totals      map[string]any   `json:"totals"`
			Today       map[string]any   `json:"today"`
			Daily       []map[string]any `json:"daily"`
			ByModel     []map[string]any `json:"by_model"`
			ModelsDaily []map[string]any `json:"models_daily"`
			Recent      []map[string]any `json:"recent"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json: %v", err)
	}
	if payload.Data.Days != 7 {
		t.Fatalf("days = %d, want 7", payload.Data.Days)
	}
	if payload.Data.Totals["requests"].(float64) != 3 {
		t.Fatalf("totals.requests = %v", payload.Data.Totals["requests"])
	}
	if payload.Data.Totals["latency_p50_ms"].(float64) != 700 {
		t.Fatalf("totals.latency_p50_ms = %v", payload.Data.Totals["latency_p50_ms"])
	}
	if payload.Data.Totals["latency_p95_ms"].(float64) != 950 {
		t.Fatalf("totals.latency_p95_ms = %v", payload.Data.Totals["latency_p95_ms"])
	}
	if payload.Data.Today["requests"].(float64) != 1 {
		t.Fatalf("today.requests = %v", payload.Data.Today["requests"])
	}
	if len(payload.Data.Daily) != 1 || payload.Data.Daily[0]["date"] != "2026-09-04" {
		t.Fatalf("daily = %v", payload.Data.Daily)
	}
	byVendor, ok := payload.Data.Daily[0]["by_vendor"].(map[string]any)
	if !ok || byVendor["gemini"].(float64) != 3 {
		t.Fatalf("daily[0].by_vendor = %v", payload.Data.Daily[0]["by_vendor"])
	}
	byStatus, ok := payload.Data.Daily[0]["by_status"].(map[string]any)
	if !ok || byStatus["429"].(float64) != 1 {
		t.Fatalf("daily[0].by_status = %v", payload.Data.Daily[0]["by_status"])
	}
	if len(payload.Data.ByModel) != 1 || payload.Data.ByModel[0]["vendor"] != "gemini" {
		t.Fatalf("by_model = %v", payload.Data.ByModel)
	}
	if len(payload.Data.ModelsDaily) != 1 || payload.Data.ModelsDaily[0]["vendor"] != "gemini" {
		t.Fatalf("models_daily = %v", payload.Data.ModelsDaily)
	}
	modelDaily, ok := payload.Data.ModelsDaily[0]["daily"].([]any)
	if !ok || len(modelDaily) != 1 {
		t.Fatalf("models_daily[0].daily = %v", payload.Data.ModelsDaily[0]["daily"])
	}
	if len(payload.Data.Recent) != 1 || payload.Data.Recent[0]["op"] != "generate" {
		t.Fatalf("recent = %v", payload.Data.Recent)
	}
}
