package toolhandlers

import (
	"context"
	"testing"

	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/application/tools"
	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
)

func TestProductCasesMapsRows(t *testing.T) {
	repo := &stubCasesRepo{casesByProduct: []reportsmysql.CaseRow{{Code: "REP-2", Title: "belt", Status: "done"}}}
	h := NewProductCases(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{
		Text:     "cases of model X",
		Coverage: []int64{3},
		Tool:     tools.Tool{Limit: 10},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0]["code"] != "REP-2" || rows[0]["title"] != "belt" || rows[0]["status"] != "done" {
		t.Fatalf("row = %v", rows[0])
	}
	if repo.byProductTerm != "cases of model X" {
		t.Fatalf("byProductTerm = %q", repo.byProductTerm)
	}
}

func TestProductCasesEmptyCoverage(t *testing.T) {
	repo := &stubCasesRepo{casesByProduct: []reportsmysql.CaseRow{{Code: "REP-2"}}}
	h := NewProductCases(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{Text: "model X", Coverage: nil})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0 on empty coverage", len(rows))
	}
	if repo.byProductCoverage != nil {
		t.Fatal("repo must not be queried on empty coverage")
	}
}
