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

func TestTrackCaseNormalizesLooseRepShapes(t *testing.T) {
	shapes := []string{"where is REP-4105", "where is rep-4105", "where is REP4105", "where is rep4105", "where is rep:4105", "where is rep 4105"}
	for _, text := range shapes {
		repo := &stubCasesRepo{caseByCode: reportsmysql.CaseRow{Code: "REP-4105", Title: "motor down", Status: "doing"}}
		h := NewTrackCase(repo)

		rows, err := h.Run(context.Background(), skeleton.Query{Text: text, Coverage: []int64{3}})
		if err != nil {
			t.Fatalf("Run(%q): %v", text, err)
		}
		if repo.byCodeArg != "REP-4105" {
			t.Fatalf("Run(%q) byCodeArg = %q, want REP-4105", text, repo.byCodeArg)
		}
		if len(rows) != 2 || rows[0]["value"] != "motor down" {
			t.Fatalf("Run(%q) rows = %v", text, rows)
		}
	}
}

func TestTrackCaseResolvesDraftRefByID(t *testing.T) {
	repo := &stubCasesRepo{caseByID: reportsmysql.CaseRow{Title: "unlinked mail", Status: ""}}
	h := NewTrackCase(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{Text: "diagnose draft45", Coverage: []int64{3}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if repo.byIDArg != 45 {
		t.Fatalf("byIDArg = %d, want 45", repo.byIDArg)
	}
	if repo.caseIDByCodeCalled {
		t.Fatal("draft refs must not go through the code lookup path")
	}
	if len(rows) != 2 || rows[0]["value"] != "unlinked mail" {
		t.Fatalf("rows = %v", rows)
	}
}
