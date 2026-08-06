package toolhandlers

import (
	"context"
	"strconv"

	"github.com/my/app/internal/application/skeleton"
)

type Workload struct {
	repo EmployeeRepo
}

func NewWorkload(repo EmployeeRepo) Workload {
	return Workload{repo: repo}
}

func (h Workload) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	limit := q.Tool.Limit
	if limit <= 0 {
		limit = 20
	}
	loads, err := h.repo.Workload(ctx, q.Coverage, limit)
	if err != nil {
		return nil, err
	}
	rows := make([]skeleton.Row, 0, len(loads))
	for _, l := range loads {
		rows = append(rows, skeleton.Row{
			"name":   l.Name,
			"open":   strconv.FormatInt(l.Open, 10),
			"oldest": l.Oldest,
		})
	}
	return rows, nil
}
