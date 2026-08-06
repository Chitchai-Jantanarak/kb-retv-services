package toolhandlers

import (
	"context"
	"testing"

	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/application/tools"
	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
)

func TestFindCasesMapsRows(t *testing.T) {
	repo := &stubCasesRepo{latestCases: []reportsmysql.CaseRow{{Code: "REP-1", Title: "broken", Status: "waiting"}}}
	h := NewFindCases(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{
		Text:     "show waiting cases",
		Coverage: []int64{3},
		Tool:     tools.Tool{Limit: 10},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0]["code"] != "REP-1" || rows[0]["title"] != "broken" || rows[0]["status"] != "waiting" {
		t.Fatalf("row = %v", rows[0])
	}
	if rows[0]["assignee"] != "" {
		t.Fatalf("assignee = %q, want empty", rows[0]["assignee"])
	}
	if repo.latestStatus != "waiting" {
		t.Fatalf("latestStatus = %q, want waiting", repo.latestStatus)
	}
	if repo.latestLimit != 10 {
		t.Fatalf("latestLimit = %d, want 10", repo.latestLimit)
	}
}

func TestFindCasesHonoursRequestedCount(t *testing.T) {
	repo := &stubCasesRepo{}
	h := NewFindCases(repo)

	if _, err := h.Run(context.Background(), skeleton.Query{
		Text:     "ขอเคสปัญหา 4 อย่างล่าสุด",
		Coverage: []int64{3},
		Tool:     tools.Tool{Limit: 10},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if repo.latestLimit != 4 {
		t.Fatalf("latestLimit = %d, want 4", repo.latestLimit)
	}
}

func TestRequestedLimit(t *testing.T) {
	cases := []struct {
		text string
		cap  int
		want int
	}{
		{"give me 3 latest incoming cases", 10, 3},
		{"ขอเคสปัญหา 4 อย่างล่าสุด", 10, 4},
		{"find cases", 10, 10},
		{"status of REP-606", 10, 10},
		{"show 25 cases", 10, 10},
		{"show 0 cases", 10, 10},
	}
	for _, c := range cases {
		if got := requestedLimit(c.text, c.cap); got != c.want {
			t.Fatalf("requestedLimit(%q, %d) = %d, want %d", c.text, c.cap, got, c.want)
		}
	}
}
