package mysql

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/my/app/internal/infra/llm"
)

func TestRequestLogRepoRecordValidatesInput(t *testing.T) {
	repo := NewRequestLogRepo(nil)
	cases := []struct {
		name    string
		log     llm.RequestLog
		wantSub string
	}{
		{name: "zero_company", log: llm.RequestLog{Vendor: "gemini"}, wantSub: "company_id"},
		{name: "missing_vendor", log: llm.RequestLog{CompanyID: 1, Vendor: "   "}, wantSub: "vendor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := repo.Record(context.Background(), tc.log)
			if err == nil {
				t.Fatalf("err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want sub %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestRequestLogRepoUsageValidatesInput(t *testing.T) {
	repo := NewRequestLogRepo(nil)
	if _, err := repo.Usage(context.Background(), 0, 7); err == nil || !strings.Contains(err.Error(), "company_id") {
		t.Fatalf("err = %v, want company_id error", err)
	}
	if _, err := repo.Usage(context.Background(), 1, 0); err == nil || !strings.Contains(err.Error(), "days") {
		t.Fatalf("err = %v, want days error", err)
	}
}

func TestFillDailyRangeFillsMissingDaysOldestToNewest(t *testing.T) {
	loc := time.UTC
	since := time.Date(2026, 8, 29, 0, 0, 0, 0, loc)
	byDate := map[string]UsageDay{
		"2026-08-30": {Date: "2026-08-30", Requests: 5, Errors: 1, InputTokens: 100, OutputTokens: 50},
	}

	got := fillDailyRange(byDate, since, 3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}

	want := []string{"2026-08-29", "2026-08-30", "2026-08-31"}
	for i, date := range want {
		if got[i].Date != date {
			t.Fatalf("got[%d].Date = %q, want %q", i, got[i].Date, date)
		}
	}

	if got[0].Requests != 0 || got[0].Errors != 0 {
		t.Fatalf("got[0] = %+v, want zero-filled", got[0])
	}
	if got[1].Requests != 5 || got[1].Errors != 1 || got[1].InputTokens != 100 || got[1].OutputTokens != 50 {
		t.Fatalf("got[1] = %+v, want the seeded row", got[1])
	}
	if got[2].Requests != 0 {
		t.Fatalf("got[2] = %+v, want zero-filled", got[2])
	}
}

func TestPercentile(t *testing.T) {
	if got := percentile(nil, 50); got != 0 {
		t.Fatalf("empty: got %d, want 0", got)
	}
	if got := percentile([]int{42}, 50); got != 42 {
		t.Fatalf("one value p50: got %d, want 42", got)
	}
	if got := percentile([]int{42}, 95); got != 42 {
		t.Fatalf("one value p95: got %d, want 42", got)
	}

	sorted := make([]int, 20)
	for i := range sorted {
		sorted[i] = (i + 1) * 10 // 10, 20, ..., 200
	}
	// nearest-rank: rank = ceil(p/100 * N)
	if got := percentile(sorted, 50); got != 100 {
		t.Fatalf("20 values p50: got %d, want 100 (10th value)", got)
	}
	if got := percentile(sorted, 95); got != 190 {
		t.Fatalf("20 values p95: got %d, want 190 (19th value)", got)
	}
}

func TestFillModelDailyRangeFillsMissingDaysOldestToNewest(t *testing.T) {
	loc := time.UTC
	since := time.Date(2026, 8, 29, 0, 0, 0, 0, loc)
	byDate := map[string]UsageModelDay{
		"2026-08-30": {Date: "2026-08-30", Requests: 4, Errors: 1, InputTokens: 80, OutputTokens: 40},
	}

	got := fillModelDailyRange(byDate, since, 3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}

	want := []string{"2026-08-29", "2026-08-30", "2026-08-31"}
	for i, date := range want {
		if got[i].Date != date {
			t.Fatalf("got[%d].Date = %q, want %q", i, got[i].Date, date)
		}
	}

	if got[0].Requests != 0 || got[0].Errors != 0 {
		t.Fatalf("got[0] = %+v, want zero-filled", got[0])
	}
	if got[1].Requests != 4 || got[1].Errors != 1 || got[1].InputTokens != 80 || got[1].OutputTokens != 40 {
		t.Fatalf("got[1] = %+v, want the seeded row", got[1])
	}
	if got[2].Requests != 0 {
		t.Fatalf("got[2] = %+v, want zero-filled", got[2])
	}
}
