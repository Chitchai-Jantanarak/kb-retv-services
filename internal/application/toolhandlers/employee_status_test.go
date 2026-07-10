package toolhandlers

import (
	"context"
	"testing"

	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/application/tools"
	employeemysql "github.com/my/app/internal/repositories/employee/mysql"
)

type stubEmployeeRepo struct {
	cases        []employeemysql.CaseRow
	casesName    string
	casesCovered []int64
	loads        []employeemysql.WorkloadRow
	loadsCovered []int64
}

func (s *stubEmployeeRepo) OpenCasesByEmployee(_ context.Context, coverage []int64, name string, _ int) ([]employeemysql.CaseRow, error) {
	s.casesCovered = coverage
	s.casesName = name
	return s.cases, nil
}

func (s *stubEmployeeRepo) Workload(_ context.Context, coverage []int64, _ int) ([]employeemysql.WorkloadRow, error) {
	s.loadsCovered = coverage
	return s.loads, nil
}

func TestEmployeeStatusMapsRows(t *testing.T) {
	repo := &stubEmployeeRepo{cases: []employeemysql.CaseRow{{Code: "REP-5", Title: "pump", Status: "doing"}}}
	h := NewEmployeeStatus(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{
		Text:     "what is Nok doing",
		Coverage: []int64{3},
		Tool:     tools.Tool{Limit: 5},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0]["code"] != "REP-5" || rows[0]["title"] != "pump" || rows[0]["status"] != "doing" {
		t.Fatalf("row = %v", rows[0])
	}
	if repo.casesName != "what is Nok doing" {
		t.Fatalf("casesName = %q", repo.casesName)
	}
}

func TestEmployeeStatusEmptyCoverage(t *testing.T) {
	repo := &stubEmployeeRepo{cases: []employeemysql.CaseRow{{Code: "REP-5"}}}
	h := NewEmployeeStatus(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{Text: "Nok", Coverage: nil})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0 on empty coverage", len(rows))
	}
	if repo.casesCovered != nil {
		t.Fatal("repo must not be queried on empty coverage")
	}
}
