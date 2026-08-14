package toolhandlers

import (
	"context"
	"testing"

	"github.com/my/app/internal/application/skeleton"
	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
)

type stubCasesRepo struct {
	caseID  int64
	caseErr error

	isResponsible    bool
	isResponsibleErr error

	updateErr error

	caseIDByCodeCoverage []int64
	caseIDByCodeCalled   bool

	updateCalled   bool
	updateCoverage []int64
	updateReportID int64
	updateStatus   string

	assignErr        error
	assignCalled     bool
	assignCoverage   []int64
	assignReportID   int64
	assignEmployeeID int64

	latestCases       []reportsmysql.CaseRow
	latestCoverage    []int64
	latestStatus      string
	latestScope       string
	latestLimit       int
	caseByCode        reportsmysql.CaseRow
	caseByCodeErr     error
	byCodeCoverage    []int64
	byCodeArg         string
	caseByID          reportsmysql.CaseRow
	caseByIDErr       error
	byIDCoverage      []int64
	byIDArg           int64
	casesByProduct    []reportsmysql.CaseRow
	byProductCoverage []int64
	byProductTerm     string
}

func (s *stubCasesRepo) IsResponsible(_ context.Context, _, _, _ int64) (bool, error) {
	return s.isResponsible, s.isResponsibleErr
}

func (s *stubCasesRepo) CaseIDByCode(_ context.Context, coverage []int64, _ string) (int64, error) {
	s.caseIDByCodeCalled = true
	s.caseIDByCodeCoverage = coverage
	if s.caseErr != nil {
		return 0, s.caseErr
	}
	return s.caseID, nil
}

func (s *stubCasesRepo) UpdateCaseStatus(_ context.Context, coverage []int64, reportID int64, statusCode string) error {
	s.updateCalled = true
	s.updateCoverage = coverage
	s.updateReportID = reportID
	s.updateStatus = statusCode
	return s.updateErr
}

func (s *stubCasesRepo) AssignCase(_ context.Context, coverage []int64, reportID, employeeID int64) error {
	s.assignCalled = true
	s.assignCoverage = coverage
	s.assignReportID = reportID
	s.assignEmployeeID = employeeID
	return s.assignErr
}

func (s *stubCasesRepo) LatestCases(_ context.Context, coverage []int64, status string, scope string, limit int) ([]reportsmysql.CaseRow, error) {
	s.latestCoverage = coverage
	s.latestStatus = status
	s.latestScope = scope
	s.latestLimit = limit
	return s.latestCases, nil
}

func (s *stubCasesRepo) CaseByCode(_ context.Context, coverage []int64, code string) (reportsmysql.CaseRow, error) {
	s.byCodeCoverage = coverage
	s.byCodeArg = code
	if s.caseByCodeErr != nil {
		return reportsmysql.CaseRow{}, s.caseByCodeErr
	}
	return s.caseByCode, nil
}

func (s *stubCasesRepo) CaseByID(_ context.Context, coverage []int64, id int64) (reportsmysql.CaseRow, error) {
	s.byIDCoverage = coverage
	s.byIDArg = id
	if s.caseByIDErr != nil {
		return reportsmysql.CaseRow{}, s.caseByIDErr
	}
	return s.caseByID, nil
}

func (s *stubCasesRepo) CasesByProduct(_ context.Context, coverage []int64, product string, _ int) ([]reportsmysql.CaseRow, error) {
	s.byProductCoverage = coverage
	s.byProductTerm = product
	return s.casesByProduct, nil
}

func TestUpdateDeniedWhenNotResponsible(t *testing.T) {
	repo := &stubCasesRepo{caseID: 100, isResponsible: false}
	h := NewUpdateCase(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "mark case REP-100 done",
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

func TestUpdateAllowedWhenLead(t *testing.T) {
	repo := &stubCasesRepo{caseID: 100}
	h := NewUpdateCase(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "mark case REP-100 done",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.assign"}, Coverage: []int64{3}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !repo.updateCalled {
		t.Fatal("UpdateCaseStatus should be called when actor has report.assign")
	}
	if len(repo.updateCoverage) != 1 || repo.updateCoverage[0] != 3 {
		t.Fatalf("updateCoverage = %v, want [3]", repo.updateCoverage)
	}
	if repo.updateReportID != 100 {
		t.Fatalf("updateReportID = %d, want 100", repo.updateReportID)
	}
	if repo.updateStatus != "done" {
		t.Fatalf("updateStatus = %q, want done", repo.updateStatus)
	}
}

func TestUpdateAllowedWhenResponsible(t *testing.T) {
	repo := &stubCasesRepo{caseID: 100, isResponsible: true}
	h := NewUpdateCase(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "mark case REP-100 done",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.update"}, Coverage: []int64{3}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !repo.updateCalled {
		t.Fatal("UpdateCaseStatus should be called when actor is the responsible employee")
	}
}

func TestUpdateNeedsCodeAndStatus(t *testing.T) {
	repo := &stubCasesRepo{caseID: 100}
	h := NewUpdateCase(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "update the case",
		Coverage: []int64{3},
		Actor:    skeleton.Actor{UserID: 55, Perms: []string{"report.assign"}, Coverage: []int64{3}},
	})
	if err == nil {
		t.Fatal("err = nil, want error when code and status are missing")
	}
	if repo.caseIDByCodeCalled || repo.updateCalled {
		t.Fatal("repo must not be touched when the message lacks a code/status")
	}
}

func TestUpdateUsesCoverageNotMessage(t *testing.T) {
	repo := &stubCasesRepo{caseID: 100}
	h := NewUpdateCase(repo)

	_, err := h.Run(context.Background(), skeleton.Query{
		Text:     "in company 99 mark case REP-100 done",
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
