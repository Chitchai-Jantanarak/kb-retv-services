package toolhandlers

import (
	"context"
	"strings"

	"github.com/my/app/internal/application/skeleton"
)

type ProductCases struct {
	repo CasesRepo
}

func NewProductCases(repo CasesRepo) ProductCases {
	return ProductCases{repo: repo}
}

func (h ProductCases) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	if len(q.Coverage) == 0 {
		return []skeleton.Row{}, nil
	}
	limit := q.Tool.Limit
	if limit <= 0 {
		limit = 10
	}
	cases, err := h.repo.CasesByProduct(ctx, q.Coverage, strings.TrimSpace(q.Text), limit)
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
