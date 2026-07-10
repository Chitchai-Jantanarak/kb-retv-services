package toolhandlers

import (
	"context"
	"testing"

	"github.com/my/app/internal/application/skeleton"
	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
)

func TestTrackCaseReturnsFieldRows(t *testing.T) {
	repo := &stubCasesRepo{caseByCode: reportsmysql.CaseRow{Code: "REP-9", Title: "motor down", Status: "doing"}}
	h := NewTrackCase(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{
		Text:     "where is REP-9",
		Coverage: []int64{3},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0]["field"] != "title" || rows[0]["value"] != "motor down" {
		t.Fatalf("row0 = %v", rows[0])
	}
	if rows[1]["field"] != "status" || rows[1]["value"] != "doing" {
		t.Fatalf("row1 = %v", rows[1])
	}
	if len(repo.byCodeCoverage) != 1 || repo.byCodeCoverage[0] != 3 {
		t.Fatalf("byCodeCoverage = %v, want [3]", repo.byCodeCoverage)
	}
}

func TestTrackCaseNeedsCode(t *testing.T) {
	repo := &stubCasesRepo{}
	h := NewTrackCase(repo)

	_, err := h.Run(context.Background(), skeleton.Query{Text: "where is my ticket", Coverage: []int64{3}})
	if err == nil {
		t.Fatal("err = nil, want error when code is missing")
	}
}

func TestTrackCaseEmptyCoverage(t *testing.T) {
	repo := &stubCasesRepo{caseByCode: reportsmysql.CaseRow{Code: "REP-9"}}
	h := NewTrackCase(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{Text: "where is REP-9", Coverage: nil})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0 on empty coverage", len(rows))
	}
	if repo.byCodeCoverage != nil {
		t.Fatal("repo must not be queried on empty coverage")
	}
}
