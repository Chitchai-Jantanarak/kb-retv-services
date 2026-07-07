package mysql

import (
	"context"
	"strings"
	"testing"
)

func TestBuildListInboundDraftsQueryScopesToCoverage(t *testing.T) {
	query, args := buildListInboundDraftsQuery([]int64{3, 4}, 10)
	if !strings.Contains(query, "company_id IN (?,?)") {
		t.Fatalf("query = %q, want company_id IN (?,?)", query)
	}
	if !strings.Contains(query, "status = 'pending'") {
		t.Fatalf("query = %q, want status = 'pending' filter", query)
	}
	if len(args) != 3 || args[0] != int64(3) || args[1] != int64(4) || args[2] != 10 {
		t.Fatalf("args = %v, want [3 4 10]", args)
	}
}

func TestBuildListInboundDraftsQueryDefaultsLimit(t *testing.T) {
	_, args := buildListInboundDraftsQuery([]int64{3}, 0)
	if len(args) != 2 || args[1] != 10 {
		t.Fatalf("args = %v, want default limit 10", args)
	}
}

func TestListInboundDraftsEmptyCoverageRunsNoQuery(t *testing.T) {
	q := &fakeQuerier{}
	repo := New(q)

	rows, err := repo.ListInboundDrafts(context.Background(), nil, 10)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %v, want empty slice", rows)
	}
	if q.queryCalled {
		t.Fatal("QueryContext must not be called when coverage is empty")
	}
}
