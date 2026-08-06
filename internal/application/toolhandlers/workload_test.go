package toolhandlers

import (
	"context"
	"testing"

	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/application/tools"
	employeemysql "github.com/my/app/internal/repositories/employee/mysql"
)

func TestWorkloadMapsRows(t *testing.T) {
	repo := &stubEmployeeRepo{loads: []employeemysql.WorkloadRow{{Name: "Nok", Open: 4, Oldest: "2026-01-02"}}}
	h := NewWorkload(repo)

	rows, err := h.Run(context.Background(), skeleton.Query{
		Text:     "team workload",
		Coverage: []int64{3},
		Tool:     tools.Tool{Limit: 20},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0]["name"] != "Nok" || rows[0]["open"] != "4" || rows[0]["oldest"] != "2026-01-02" {
		t.Fatalf("row = %v", rows[0])
	}
}
