package skeleton

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/my/app/internal/application/tools"
)

type Actor struct {
	CompanyID int64
	UserID    int64
	Perms     []string
	Coverage  []int64
}

type Query struct {
	Tool     tools.Tool
	Text     string
	Coverage []int64
	Actor    Actor
}

type Row map[string]string

type Handler interface {
	Run(ctx context.Context, q Query) ([]Row, error)
}

type ToolSelector interface {
	Select(ctx context.Context, text string, perms []string) (tools.Selection, error)
}

type Auditor interface {
	Log(ctx context.Context, tag string, actor Actor, toolID string) error
}

type Response struct {
	Matched  bool
	ToolID   string
	Headline string
	Lines    []string
}

type Orchestrator struct {
	selector ToolSelector
	catalog  map[string]tools.Tool
	handlers map[string]Handler
	auditor  Auditor
}

func New(selector ToolSelector, catalog []tools.Tool, handlers map[string]Handler, auditor Auditor) *Orchestrator {
	byID := make(map[string]tools.Tool, len(catalog))
	for _, t := range catalog {
		byID[t.ID] = t
	}
	return &Orchestrator{selector: selector, catalog: byID, handlers: handlers, auditor: auditor}
}

func (o *Orchestrator) Handle(ctx context.Context, actor Actor, msg string) (Response, error) {
	sel, err := o.selector.Select(ctx, msg, actor.Perms)
	if err != nil {
		return Response{}, fmt.Errorf("skeleton: select: %w", err)
	}
	if !sel.Matched {
		return Response{Matched: false}, nil
	}

	tool, ok := o.catalog[sel.ToolID]
	if !ok {
		return Response{}, fmt.Errorf("skeleton: unknown tool %q", sel.ToolID)
	}

	handler, ok := o.handlers[tool.Handler]
	if !ok {
		return Response{}, fmt.Errorf("skeleton: no handler %q for tool %q", tool.Handler, tool.ID)
	}

	rows, err := handler.Run(ctx, Query{Tool: tool, Text: msg, Coverage: actor.Coverage, Actor: actor})
	if err != nil {
		return Response{}, fmt.Errorf("skeleton: handler %q: %w", tool.Handler, err)
	}

	resp := compose(tool, rows)

	if err := o.auditor.Log(ctx, tool.Audit, actor, tool.ID); err != nil {
		return Response{}, fmt.Errorf("skeleton: audit: %w", err)
	}

	return resp, nil
}

func compose(tool tools.Tool, rows []Row) Response {
	headline := strings.ReplaceAll(tool.Compose.Headline, "{n}", strconv.Itoa(len(rows)))

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		line := tool.Compose.Row
		for k, v := range row {
			line = strings.ReplaceAll(line, "{"+k+"}", v)
		}
		lines = append(lines, line)
	}

	return Response{Matched: true, ToolID: tool.ID, Headline: headline, Lines: lines}
}
