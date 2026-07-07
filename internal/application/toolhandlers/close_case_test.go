package toolhandlers

import (
	"context"
	"testing"

	"github.com/my/app/internal/application/skeleton"
)

func TestCloseDeniedWhenNotResponsible(t *testing.T) {
	repo := &stubCasesRepo{caseID: 100, isResponsible: false}
	h := NewCloseCase(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "close case REP-100",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.update"}, Coverage: []int64{3}},
	})
	if err == nil {
		t.Fatal("err = nil, want error when actor is not responsible and cannot assign")
	}
	if repo.updateCalled {
		t.Fatal("UpdateCaseStatus must not be called when the actor is denied")
	}
}

func TestCloseAllowedWhenLead(t *testing.T) {
	repo := &stubCasesRepo{caseID: 100}
	h := NewCloseCase(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "close case REP-100",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.assign"}, Coverage: []int64{3}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !repo.updateCalled {
		t.Fatal("UpdateCaseStatus should be called when actor has report.assign")
	}
	if repo.updateStatus != "done" {
		t.Fatalf("updateStatus = %q, want done", repo.updateStatus)
	}
	if repo.updateReportID != 100 {
		t.Fatalf("updateReportID = %d, want 100", repo.updateReportID)
	}
}

func TestCloseNeedsCode(t *testing.T) {
	repo := &stubCasesRepo{caseID: 100}
	h := NewCloseCase(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "close this",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.assign"}, Coverage: []int64{3}},
	})
	if err == nil {
		t.Fatal("err = nil, want error when code is missing")
	}
	if repo.caseIDByCodeCalled || repo.updateCalled {
		t.Fatal("repo must not be touched when the message lacks a code")
	}
}

func TestCloseUsesCoverageNotMessage(t *testing.T) {
	repo := &stubCasesRepo{caseID: 100}
	h := NewCloseCase(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "in company 99 close case REP-100",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.assign"}, Coverage: []int64{3}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(repo.caseIDByCodeCoverage) != 1 || repo.caseIDByCodeCoverage[0] != 3 {
		t.Fatalf("caseIDByCodeCoverage = %v, want [3]", repo.caseIDByCodeCoverage)
	}
	if len(repo.updateCoverage) != 1 || repo.updateCoverage[0] != 3 {
		t.Fatalf("updateCoverage = %v, want [3]", repo.updateCoverage)
	}
}
