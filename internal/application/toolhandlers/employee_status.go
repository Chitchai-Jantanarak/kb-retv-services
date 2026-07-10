package toolhandlers

import (
	"context"
	"strings"

	"github.com/my/app/internal/application/skeleton"
	employeemysql "github.com/my/app/internal/repositories/employee/mysql"
)

type EmployeeRepo interface {
	OpenCasesByEmployee(ctx context.Context, coverage []int64, name string, limit int) ([]employeemysql.CaseRow, error)
	Workload(ctx context.Context, coverage []int64, limit int) ([]employeemysql.WorkloadRow, error)
}

type EmployeeStatus struct {
	repo EmployeeRepo
}

func NewEmployeeStatus(repo EmployeeRepo) EmployeeStatus {
	return EmployeeStatus{repo: repo}
}

func (h EmployeeStatus) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	if len(q.Coverage) == 0 {
		return []skeleton.Row{}, nil
	}
	limit := q.Tool.Limit
	if limit <= 0 {
		limit = 5
	}
	cases, err := h.repo.OpenCasesByEmployee(ctx, q.Coverage, strings.TrimSpace(q.Text), limit)
	if err != nil {
		return nil, err
	}
	rows := make([]skeleton.Row, 0, len(cases))
	for _, c := range cases {
		rows = append(rows, skeleton.Row{
			"code":   c.Code,
			"title":  c.Title,
			"status": c.Status,
		})
	}
	return rows, nil
}
