package toolhandlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/my/app/internal/application/skeleton"
)

type CloseCase struct {
	repo CasesRepo
}

func NewCloseCase(repo CasesRepo) CloseCase {
	return CloseCase{repo: repo}
}

func (h CloseCase) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	code := strings.ToUpper(caseCodeRe.FindString(q.Text))
	if code == "" {
		return nil, fmt.Errorf("close_case: a case code is required")
	}
	id, err := h.repo.CaseIDByCode(ctx, q.Coverage, code)
	if err != nil {
		return nil, err
	}
	if !CanMutate(ctx, h.repo, q.Actor, id) {
		return nil, fmt.Errorf("close_case: not permitted for this case")
	}
	if err := h.repo.UpdateCaseStatus(ctx, q.Coverage, id, "done"); err != nil {
		return nil, err
	}
	return []skeleton.Row{{"code": code, "status": "done"}}, nil
}
