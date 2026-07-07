package toolhandlers

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/my/app/internal/application/skeleton"
)

var assignEmployeeRe = regexp.MustCompile(`(?i)(?:to|ให้)\s*#?(\d+)`)

type AssignCase struct {
	repo CasesRepo
}

func NewAssignCase(repo CasesRepo) AssignCase {
	return AssignCase{repo: repo}
}

func (h AssignCase) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	code := strings.ToUpper(caseCodeRe.FindString(q.Text))
	employeeID := parseEmployeeID(q.Text)
	if code == "" || employeeID == 0 {
		return nil, fmt.Errorf("assign_case: a case code and a numeric employee id are required")
	}
	id, err := h.repo.CaseIDByCode(ctx, q.Coverage, code)
	if err != nil {
		return nil, err
	}
	if !CanMutate(ctx, h.repo, q.Actor, id) {
		return nil, fmt.Errorf("assign_case: not permitted for this case")
	}
	if err := h.repo.AssignCase(ctx, q.Coverage, id, employeeID); err != nil {
		return nil, err
	}
	return []skeleton.Row{{"code": code, "employee_id": strconv.FormatInt(employeeID, 10)}}, nil
}

func parseEmployeeID(text string) int64 {
	m := assignEmployeeRe.FindStringSubmatch(text)
	if len(m) < 2 {
		return 0
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0
	}
	return id
}
