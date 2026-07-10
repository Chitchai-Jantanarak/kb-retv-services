package toolhandlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/my/app/internal/application/skeleton"
)

type TrackCase struct {
	repo CasesRepo
}

func NewTrackCase(repo CasesRepo) TrackCase {
	return TrackCase{repo: repo}
}

func (h TrackCase) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	if len(q.Coverage) == 0 {
		return []skeleton.Row{}, nil
	}
	code := strings.ToUpper(caseCodeRe.FindString(q.Text))
	if code == "" {
		return nil, fmt.Errorf("track_case: a case code is required")
	}
	c, err := h.repo.CaseByCode(ctx, q.Coverage, code)
	if err != nil {
		return nil, err
	}
	return []skeleton.Row{
		{"field": "title", "value": c.Title},
		{"field": "status", "value": c.Status},
	}, nil
}
