package toolhandlers

import (
	"context"

	"github.com/my/app/internal/application/skeleton"
)

type FindCases struct {
	repo CasesRepo
}

func NewFindCases(repo CasesRepo) FindCases {
	return FindCases{repo: repo}
}

func (h FindCases) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	if len(q.Coverage) == 0 {
		return []skeleton.Row{}, nil
	}
	limit := q.Tool.Limit
	if limit <= 0 {
		limit = 10
	}
	cases, err := h.repo.LatestCases(ctx, q.Coverage, parseStatus(q.Text), limit)
	if err != nil {
		return nil, err
	}
	rows := make([]skeleton.Row, 0, len(cases))
	for _, c := range cases {
		rows = append(rows, skeleton.Row{
			"code":     c.Code,
			"title":    c.Title,
			"status":   c.Status,
			"assignee": "",
		})
	}
	return rows, nil
}
