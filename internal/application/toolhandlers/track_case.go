package toolhandlers

import (
	"context"
	"strconv"

	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/shared/debugtrace"
)

type TrackCase struct {
	repo CasesRepo
}

func NewTrackCase(repo CasesRepo) TrackCase {
	return TrackCase{repo: repo}
}

func (h TrackCase) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	code := paramOrParse(q, "code")
	if code == "" {
		debugtrace.Add(ctx, debugtrace.Event{
			Stage: "handler.params",
			State: "missing",
			Label: "resolve case lookup parameters",
			Context: map[string]string{
				"required":       "code",
				"coverage_count": strconv.Itoa(len(q.Coverage)),
			},
		})
		return nil, skeleton.NeedsParams("case_code")
	}
	source := "message pattern"
	if q.Params["code"] != "" {
		source = "tool selector"
	}
	debugtrace.Add(ctx, debugtrace.Event{
		Stage: "handler.params",
		State: "resolved",
		Label: "resolve case lookup parameters",
		Context: map[string]string{
			"code":           code,
			"code_source":    source,
			"coverage_count": strconv.Itoa(len(q.Coverage)),
			"repository":     "reports/mysql.CaseByCode",
		},
	})
	c, err := h.repo.CaseByCode(ctx, q.Coverage, code)
	if err != nil {
		return nil, err
	}
	return []skeleton.Row{
		{"field": "title", "value": c.Title},
		{"field": "status", "value": c.Status},
	}, nil
}
