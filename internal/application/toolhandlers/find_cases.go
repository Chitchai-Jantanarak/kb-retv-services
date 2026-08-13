package toolhandlers

import (
	"context"
	"regexp"
	"strconv"

	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/application/tools"
)

var countRe = regexp.MustCompile(`\b([1-9][0-9]?)\b`)

func requestedLimit(text string, cap int) int {
	cleaned := tools.StripCaseRefs(text)
	m := countRe.FindStringSubmatch(cleaned)
	if m == nil {
		return cap
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 || n > cap {
		return cap
	}
	return n
}

type FindCases struct {
	repo CasesRepo
}

func NewFindCases(repo CasesRepo) FindCases {
	return FindCases{repo: repo}
}

func (h FindCases) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	limit := q.Tool.Limit
	if limit <= 0 {
		limit = 10
	}
	limit = requestedLimit(q.Text, limit)
	status := q.Params["status"]
	if status == "" {
		status = parseStatus(q.Text)
	}
	cases, err := h.repo.LatestCases(ctx, q.Coverage, status, limit)
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
