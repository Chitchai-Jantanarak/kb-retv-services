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

func TestFindCasesEmptyCoverage(t *testing.T) {
	repo := &stubCasesRepo{latestCases: []reportsmysql.CaseRow{{Code: "REP-1"}}}
	h := NewFindCases(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{Text: "cases", Coverage: nil})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0 on empty coverage", len(rows))
	}
	if repo.latestCoverage != nil {
		t.Fatal("repo must not be queried on empty coverage")
	}
}
