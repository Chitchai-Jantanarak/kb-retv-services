package toolhandlers

import (
	"context"
	"strconv"

	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/application/tools"
	reportsmysql "github.com/my/app/internal/repositories/reports/mysql"
	"github.com/my/app/internal/shared/debugtrace"
)

type TrackCase struct {
	repo CasesRepo
}

func NewTrackCase(repo CasesRepo) TrackCase {
	return TrackCase{repo: repo}
}

func (h TrackCase) Run(ctx context.Context, q skeleton.Query) ([]skeleton.Row, error) {
	ref, ok := caseRefOrParse(q)
	if !ok {
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
	repository := "reports/mysql.CaseByCode"
	if ref.Namespace == tools.CaseRefDraft {
		repository = "reports/mysql.CaseByID"
	}
	debugtrace.Add(ctx, debugtrace.Event{
		Stage: "handler.params",
		State: "resolved",
		Label: "resolve case lookup parameters",
		Context: map[string]string{
			"code":           ref.Code,
			"code_source":    source,
			"coverage_count": strconv.Itoa(len(q.Coverage)),
			"repository":     repository,
		},
	})
	var c reportsmysql.CaseRow
	var err error
	if ref.Namespace == tools.CaseRefDraft {
		c, err = h.repo.CaseByID(ctx, q.Coverage, ref.ID)
	} else {
		c, err = h.repo.CaseByCode(ctx, q.Coverage, ref.Code)
	}
	if err != nil {
		return nil, err
	}
	return []skeleton.Row{
		{"field": "title", "value": c.Title},
		{"field": "status", "value": c.Status},
	}, nil
}
