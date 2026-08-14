package decide

import (
	"context"
	"errors"

	"github.com/my/app/internal/application/intent"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/shared/ctxkey"
)

type Kind string

const (
	KindTool             Kind = "tool"
	KindClarify          Kind = "clarify"
	KindToolFailed       Kind = "tool_failed"
	KindPermissionDenied Kind = "permission_denied"
	KindHandoff          Kind = "handoff"
	KindSocial           Kind = "social"
	KindOffTopic         Kind = "off_topic"
	KindGenerative       Kind = "generative"
)

type Runner interface {
	Handle(ctx context.Context, actor skeleton.Actor, msg string) (skeleton.Response, error)
}

type Input struct {
	Actor          skeleton.Actor
	Text           string
	CiteCandidates []string
}

type Outcome struct {
	Kind     Kind
	Decision intent.Decision
	Tool     *skeleton.Response
	ToolErr  error
	Missing  []string
}

type Decider struct {
	Router         *intent.Router
	Runner         Runner
	OffTopicMargin float64
}

func (d Decider) Decide(ctx context.Context, in Input, timed func(stage string, fn func() error) error) (Outcome, error) {
	var decision intent.Decision
	err := timed("router", func() error {
		var routeErr error
		decision, routeErr = d.Router.Route(ctx, in.Text)
		return routeErr
	})
	if err != nil {
		return Outcome{}, err
	}

	out := Outcome{Decision: decision}

	if d.Runner != nil && decision.Intent != intent.Social {
		toolCtx := ctxkey.WithCiteCandidates(ctx, in.CiteCandidates)
		toolCtx = ctxkey.WithQueryVector(toolCtx, decision.Vector)
		toolCtx = ctxkey.WithRouterIntent(toolCtx, string(decision.Intent))
		var toolResp skeleton.Response
		_ = timed("tool", func() error {
			var handleErr error
			toolResp, handleErr = d.Runner.Handle(toolCtx, in.Actor, in.Text)
			out.ToolErr = handleErr
			return nil
		})
		out.Tool = &toolResp
	}

	out.Kind, out.Missing = classify(decision, out.Tool, out.ToolErr, d.OffTopicMargin)
	return out, nil
}

func classify(decision intent.Decision, tool *skeleton.Response, toolErr error, offTopicMargin float64) (Kind, []string) {
	if tool != nil {
		if toolErr != nil && errors.Is(toolErr, skeleton.ErrNeedsParam) {
			return KindClarify, missingFields(toolErr)
		}
		if toolErr != nil && (tool.Kind == "write" || Authoritative(decision.Intent)) {
			return KindToolFailed, nil
		}
		if toolErr == nil && tool.Matched {
			return KindTool, nil
		}
		if toolErr == nil && !tool.Matched && len(tool.MissingParams) > 0 {
			return KindClarify, tool.MissingParams
		}
		if Authoritative(decision.Intent) {
			if tool.PermFiltered {
				return KindPermissionDenied, nil
			}
			return KindClarify, nil
		}
	}
	switch decision.Intent {
	case intent.Handoff:
		return KindHandoff, nil
	case intent.Social:
		return KindSocial, nil
	case intent.OffDomain:
		if offTopicMargin > 0 && decision.OffMargin >= offTopicMargin {
			return KindOffTopic, nil
		}
		return KindGenerative, nil
	default:
		return KindGenerative, nil
	}
}

func Authoritative(i intent.Intent) bool {
	switch i {
	case intent.CaseStatus, intent.OpenCase:
		return true
	default:
		return false
	}
}

func missingFields(err error) []string {
	var mp skeleton.MissingParams
	if errors.As(err, &mp) {
		return mp.Fields
	}
	return nil
}
