package mysql

import (
	"context"
	"strings"
	"testing"
)

func TestBuildSelectPromotableConversationQueryScopesToCoverage(t *testing.T) {
	query, args := buildSelectPromotableConversationQuery([]int64{3, 4}, 100)
	if !strings.Contains(query, "c.company_id IN (?,?)") {
		t.Fatalf("query = %q, want company_id IN (?,?)", query)
	}
	if !strings.Contains(query, "c.status = 'pending'") {
		t.Fatalf("query = %q, want status = pending guard", query)
	}
	if len(args) != 3 {
		t.Fatalf("args = %v, want 3 args (conversation_id + 2 coverage)", args)
	}
	if args[0] != int64(100) {
		t.Fatalf("args[0] = %v, want conversation_id 100", args[0])
	}
	if args[1] != int64(3) || args[2] != int64(4) {
		t.Fatalf("args[1:] = %v, want coverage [3 4]", args[1:])
	}
}

func TestBuildMarkConversationPromotedQueryScopesToCoverage(t *testing.T) {
	query, args := buildMarkConversationPromotedQuery([]int64{3, 4}, 100, 55)
	if !strings.Contains(query, "company_id IN (?,?)") {
		t.Fatalf("query = %q, want company_id IN (?,?)", query)
	}
	if !strings.Contains(query, "status = 'promoted'") {
		t.Fatalf("query = %q, want status = promoted", query)
	}
	if len(args) != 4 {
		t.Fatalf("args = %v, want 4 args (report_id + conversation_id + 2 coverage)", args)
	}
	if args[0] != int64(55) || args[1] != int64(100) {
		t.Fatalf("args = %v, want [55 100 3 4]", args)
	}
	if args[2] != int64(3) || args[3] != int64(4) {
		t.Fatalf("args = %v, want coverage [3 4] appended", args)
	}
}

func TestPromoteConversationEmptyCoverageReturnsErrorNoQuery(t *testing.T) {
	q := &fakeCasesQuerier{}
	repo := New(q)

	_, err := repo.PromoteConversation(context.Background(), nil, 100, 55)
	if err == nil {
		t.Fatal("err = nil, want error for empty coverage")
	}
	if q.queryCalled || q.execCalled {
		t.Fatal("no query/exec must run when coverage is empty")
	}
}

func TestPromoteConversationRejectsNonPositiveIDs(t *testing.T) {
	q := &fakeCasesQuerier{}
	repo := New(q)

	if _, err := repo.PromoteConversation(context.Background(), []int64{3}, 0, 55); err == nil {
		t.Fatal("err = nil, want error for non-positive conversation_id")
	}
	if _, err := repo.PromoteConversation(context.Background(), []int64{3}, 100, 0); err == nil {
		t.Fatal("err = nil, want error for non-positive actor_employee_id")
	}
	if q.queryCalled || q.execCalled {
		t.Fatal("no query/exec must run when ids are invalid")
	}
}
