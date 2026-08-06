package toolhandlers

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/my/app/internal/application/skeleton"
	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
)

type CasesRepo interface {
	OwnershipRepo
	CaseIDByCode(ctx context.Context, coverage []int64, code string) (int64, error)
	UpdateCaseStatus(ctx context.Context, coverage []int64, reportID int64, statusCode string) error
	AssignCase(ctx context.Context, coverage []int64, reportID, employeeID int64) error
	LatestCases(ctx context.Context, coverage []int64, status string, limit int) ([]reportsmysql.CaseRow, error)
	CaseByCode(ctx context.Context, coverage []int64, code string) (reportsmysql.CaseRow, error)
	CasesByProduct(ctx context.Context, coverage []int64, product string, limit int) ([]reportsmysql.CaseRow, error)
}

var caseCodeRe = regexp.MustCompile(`(?i)REP-?\d+`)

func paramOrParse(q skeleton.Query, name string) string {
	if v := q.Params[name]; v != "" {
		return v
	}
	return strings.ToUpper(caseCodeRe.FindString(q.Text))
}

type UpdateCase struct {
	repo CasesRepo
}

func NewUpdateCase(repo CasesRepo) UpdateCase {
	return UpdateCase{repo: repo}
}

func (h UpdateCase) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	code := paramOrParse(q, "code")
	status := q.Params["status"]
	if status == "" {
		status = parseStatus(q.Text)
	}
	if code == "" || status == "" {
		return nil, skeleton.NeedsParams("case_code", "status")
	}
	id, err := h.repo.CaseIDByCode(ctx, q.Coverage, code)
	if err != nil {
		return nil, err
	}
	if !CanMutate(ctx, h.repo, q.Actor, id) {
		return nil, fmt.Errorf("update_case: not permitted for this case")
	}
	if err := h.repo.UpdateCaseStatus(ctx, q.Coverage, id, status); err != nil {
		return nil, err
	}
	return []skeleton.Row{{"code": code, "status": status}}, nil
}

func parseStatus(text string) string {
	t := strings.ToLower(text)
	for _, s := range []string{"waiting", "doing", "done"} {
		if strings.Contains(t, s) {
			return s
		}
	}
	if strings.Contains(t, "ปิด") || strings.Contains(t, "เสร็จ") {
		return "done"
	}
	return ""
}
