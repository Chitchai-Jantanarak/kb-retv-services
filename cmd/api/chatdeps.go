package main

import (
	"context"

	"github.com/my/app/internal/application/dto"
	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
)

type chatCaseSearch struct {
	repo *reportsmysql.Repository
}

func (c chatCaseSearch) SearchCases(ctx context.Context, coverage []int64, query, product, status string, limit int) ([]dto.ChatCaseResult, error) {
	rows, err := c.repo.SearchCases(ctx, coverage, query, product, status, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ChatCaseResult, 0, len(rows))
	for _, row := range rows {
		out = append(out, dto.ChatCaseResult{
			ID:     row.ID,
			Code:   row.Code,
			Title:  row.Title,
			Status: row.Status,
		})
	}
	return out, nil
}
