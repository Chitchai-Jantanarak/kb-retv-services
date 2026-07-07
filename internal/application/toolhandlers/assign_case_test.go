package toolhandlers

import (
	"context"
	"testing"

	"github.com/my/app/internal/application/skeleton"
)

func TestAssignDeniedWhenNotResponsible(t *testing.T) {
	repo := &stubCasesRepo{caseID: 100, isResponsible: false}
	h := NewAssignCase(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "assign case REP-100 to 42",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.update"}, Coverage: []int64{3}},
	})
	if err == nil {
		t.Fatal("err = nil, want error when actor is not responsible and cannot assign")
	}
	if repo.assignCalled {
		t.Fatal("AssignCase must not be called when the actor is denied")
	}
}

func TestAssignAllowedWhenLead(t *testing.T) {
	repo := &stubCasesRepo{caseID: 100}
	h := NewAssignCase(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "assign case REP-100 to 42",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.assign"}, Coverage: []int64{3}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !repo.assignCalled {
		t.Fatal("AssignCase should be called when actor has report.assign")
	}
	if repo.assignReportID != 100 {
		t.Fatalf("assignReportID = %d, want 100", repo.assignReportID)
	}
	if repo.assignEmployeeID != 42 {
		t.Fatalf("assignEmployeeID = %d, want 42", repo.assignEmployeeID)
	}
	if len(repo.assignCoverage) != 1 || repo.assignCoverage[0] != 3 {
		t.Fatalf("assignCoverage = %v, want [3]", repo.assignCoverage)
	}
}

func TestAssignNeedsCodeAndEmployee(t *testing.T) {
	repo := &stubCasesRepo{caseID: 100}
	h := NewAssignCase(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "assign this case",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.assign"}, Coverage: []int64{3}},
	})
	if err == nil {
		t.Fatal("err = nil, want error when code and employee id are missing")
	}
	if repo.caseIDByCodeCalled || repo.assignCalled {
		t.Fatal("repo must not be touched when the message lacks a code/employee id")
	}
}

func TestAssignUsesCoverageNotMessage(t *testing.T) {
	repo := &stubCasesRepo{caseID: 100}
	h := NewAssignCase(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "in company 99 assign case REP-100 to 42",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.assign"}, Coverage: []int64{3}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(repo.caseIDByCodeCoverage) != 1 || repo.caseIDByCodeCoverage[0] != 3 {
		t.Fatalf("caseIDByCodeCoverage = %v, want [3]", repo.caseIDByCodeCoverage)
	}
	if len(repo.assignCoverage) != 1 || repo.assignCoverage[0] != 3 {
		t.Fatalf("assignCoverage = %v, want [3]", repo.assignCoverage)
	}
}
