package skeleton

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/my/app/internal/application/tools"
	"github.com/my/app/internal/shared/debugtrace"
	"github.com/my/app/internal/shared/logger"
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
	Params   map[string]string
	Coverage []int64
	Actor    Actor
}

var ErrNeedsParam = errors.New("skeleton: required parameter missing")

type MissingParams struct {
	Fields []string
}

func (e MissingParams) Error() string {
	return "skeleton: missing " + strings.Join(e.Fields, ", ")
}

func (e MissingParams) Unwrap() error { return ErrNeedsParam }

func NeedsParams(fields ...string) error { return MissingParams{Fields: fields} }

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

type ToolTable struct {
	ToolID  string              `json:"tool_id"`
	Columns []tools.ToolColumn  `json:"columns"`
	Rows    []map[string]string `json:"rows"`
}

type Response struct {
	Matched            bool
	Called             bool
	ToolID             string
	Handler            string
	Kind               string
	RequiredPermission string
	Params             map[string]string
	SelectionScore     float64
	RowCount           int
	Incomplete         bool
	Headline           string
	Lines              []string
	Table              *ToolTable
	ComposeMode        string
	PermFiltered       bool
	Pending            *PendingRef
	Cite               string
	CiteValues         []string
}

type Orchestrator struct {
	selector ToolSelector
	catalog  map[string]tools.Tool
	handlers map[string]Handler
	auditor  Auditor
	pending  *PendingStore
}

type Option func(*Orchestrator)

func WithPendingStore(store *PendingStore) Option {
	return func(o *Orchestrator) { o.pending = store }
}

func New(selector ToolSelector, catalog []tools.Tool, handlers map[string]Handler, auditor Auditor, opts ...Option) *Orchestrator {
	byID := make(map[string]tools.Tool, len(catalog))
	for _, t := range catalog {
		byID[t.ID] = t
	}
	o := &Orchestrator{selector: selector, catalog: byID, handlers: handlers, auditor: auditor}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

func (o *Orchestrator) Handle(ctx context.Context, actor Actor, msg string) (Response, error) {
	sel, err := o.selector.Select(ctx, msg, actor.Perms)
	if err != nil {
		return Response{}, fmt.Errorf("skeleton: select: %w", err)
	}
	logger.FromContext(ctx).Info("skeleton: tool selection",
		zap.String("message", msg),
		zap.String("tool_id", sel.ToolID),
		zap.Float64("score", sel.Score),
		zap.Bool("matched", sel.Matched),
	)
	if !sel.Matched {
		requiredPermission := ""
		if candidate, ok := o.catalog[sel.ToolID]; ok {
			requiredPermission = candidate.RBAC.RequiresPermission
		}
		return Response{
			Matched:            false,
			ToolID:             sel.ToolID,
			RequiredPermission: requiredPermission,
			Params:             cloneParams(sel.Params),
			SelectionScore:     sel.Score,
			PermFiltered:       sel.PermFiltered,
		}, nil
	}

	tool, ok := o.catalog[sel.ToolID]
	if !ok {
		return Response{
			Matched:        true,
			ToolID:         sel.ToolID,
			Params:         cloneParams(sel.Params),
			SelectionScore: sel.Score,
		}, fmt.Errorf("skeleton: unknown tool %q", sel.ToolID)
	}
	resp := Response{
		Matched:            true,
		ToolID:             tool.ID,
		Handler:            tool.Handler,
		Kind:               tool.Kind,
		RequiredPermission: tool.RBAC.RequiresPermission,
		Params:             cloneParams(sel.Params),
		SelectionScore:     sel.Score,
	}
	requiredPermission := tool.RBAC.RequiresPermission
	if requiredPermission == "" {
		requiredPermission = "none"
	}
	debugtrace.Add(ctx, debugtrace.Event{
		Stage: "tool.catalog",
		State: "resolved",
		Label: "resolve selected tool definition",
		Context: map[string]string{
			"tool_id":             tool.ID,
			"handler":             tool.Handler,
			"operation":           strings.ToUpper(normalizedKind(tool.Kind)),
			"required_permission": requiredPermission,
			"audit_action":        tool.Audit,
		},
	})

	handler, ok := o.handlers[tool.Handler]
	if !ok {
		return resp, fmt.Errorf("skeleton: no handler %q for tool %q", tool.Handler, tool.ID)
	}

	if tool.IsWrite() && o.pending != nil {
		auditStart := time.Now()
		auditErr := o.auditor.Log(ctx, tool.Audit+".proposed", actor, tool.ID)
		recordAuditTrace(ctx, tool.Audit+".proposed", auditStart, auditErr)
		if auditErr != nil {
			return resp, fmt.Errorf("skeleton: audit: %w", auditErr)
		}
		id := o.pending.Put(actor, tool.ID, sel.Params)
		if id == "" {
			return resp, fmt.Errorf("skeleton: pending: id generation failed")
		}
		resp.Pending = &PendingRef{ID: id, Summary: proposalSummary(tool, sel.Params)}
		return resp, nil
	}

	auditStart := time.Now()
	auditErr := o.auditor.Log(ctx, tool.Audit, actor, tool.ID)
	recordAuditTrace(ctx, tool.Audit, auditStart, auditErr)
	if auditErr != nil {
		return resp, fmt.Errorf("skeleton: audit: %w", auditErr)
	}

	resp.Called = true
	handlerStart := time.Now()
	rows, err := handler.Run(ctx, Query{Tool: tool, Text: msg, Params: sel.Params, Coverage: actor.Coverage, Actor: actor})
	handlerState := "completed"
	handlerErrorCode := ""
	handlerError := ""
	if err != nil {
		handlerState = "failed"
		handlerErrorCode = "handler_failed"
		handlerError = "Handler returned an error; inspect repository events below."
	}
	debugtrace.Add(ctx, debugtrace.Event{
		Stage:      "handler." + tool.Handler,
		State:      handlerState,
		Label:      "execute " + tool.Handler,
		DurationMS: debugtrace.Since(handlerStart),
		ErrorCode:  handlerErrorCode,
		Error:      handlerError,
		Context: map[string]string{
			"component":      "internal/application/toolhandlers",
			"operation":      strings.ToUpper(normalizedKind(tool.Kind)),
			"coverage_count": strconv.Itoa(len(actor.Coverage)),
			"rows":           strconv.Itoa(len(rows)),
		},
	})
	if err != nil {
		return resp, fmt.Errorf("skeleton: handler %q: %w", tool.Handler, err)
	}

	composed := compose(tool, rows)
	composed.Called = resp.Called
	composed.Handler = resp.Handler
	composed.RequiredPermission = resp.RequiredPermission
	composed.Params = resp.Params
	composed.SelectionScore = resp.SelectionScore
	composed.RowCount = len(rows)
	return composed, nil
}

func normalizedKind(kind string) string {
	if strings.EqualFold(strings.TrimSpace(kind), "write") {
		return "write"
	}
	return "read"
}

func recordAuditTrace(ctx context.Context, action string, start time.Time, err error) {
	state := "recorded"
	errorCode := ""
	safeError := ""
	if err != nil {
		state = "failed"
		errorCode = "audit_failed"
		safeError = "Tool audit logging failed."
	}
	debugtrace.Add(ctx, debugtrace.Event{
		Stage:      "tool.audit",
		State:      state,
		Label:      "record tool audit",
		DurationMS: debugtrace.Since(start),
		ErrorCode:  errorCode,
		Error:      safeError,
		Context:    map[string]string{"action": action},
	})
}

func cloneParams(params map[string]string) map[string]string {
	if len(params) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(params))
	for key, value := range params {
		cloned[key] = value
	}
	return cloned
}

var placeholderRe = regexp.MustCompile(`\{[a-z_]+\}`)

func fillTemplate(tmpl string, row Row) string {
	for k, v := range row {
		tmpl = strings.ReplaceAll(tmpl, "{"+k+"}", v)
	}
	return tmpl
}

func tidyTemplate(s string) string {
	s = placeholderRe.ReplaceAllString(s, "")
	for strings.Contains(s, "||") {
		s = strings.ReplaceAll(s, "||", "|")
	}
	return strings.Trim(strings.TrimSpace(s), "|")
}

func compose(tool tools.Tool, rows []Row) Response {
	headline := strings.ReplaceAll(tool.Compose.Headline, "{n}", strconv.Itoa(len(rows)))
	if len(rows) > 0 {
		headline = fillTemplate(headline, rows[0])
	}
	incomplete := placeholderRe.MatchString(headline)
	headline = tidyTemplate(headline)

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, tidyTemplate(rowLine(tool, row)))
	}

	return Response{
		Matched:     true,
		ToolID:      tool.ID,
		Kind:        tool.Kind,
		Incomplete:  incomplete,
		Headline:    headline,
		Lines:       lines,
		Table:       buildTable(tool, rows),
		ComposeMode: tool.Compose.Mode,
		Cite:        tool.Compose.Cite,
		CiteValues:  citeValues(tool.Compose.Cite, rows),
	}
}

func citeValues(cite string, rows []Row) []string {
	if cite == "" {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if v := strings.TrimSpace(row[cite]); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func rowLine(tool tools.Tool, row Row) string {
	if len(tool.Compose.Columns) == 0 {
		return fillTemplate(tool.Compose.Row, row)
	}
	parts := make([]string, 0, len(tool.Compose.Columns))
	for _, col := range tool.Compose.Columns {
		parts = append(parts, row[col.Key])
	}
	return strings.Join(parts, "|")
}

func buildTable(tool tools.Tool, rows []Row) *ToolTable {
	if len(tool.Compose.Columns) == 0 {
		return nil
	}
	tableRows := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		entry := make(map[string]string, len(tool.Compose.Columns))
		for _, col := range tool.Compose.Columns {
			entry[col.Key] = row[col.Key]
		}
		tableRows = append(tableRows, entry)
	}
	return &ToolTable{ToolID: tool.ID, Columns: tool.Compose.Columns, Rows: tableRows}
}
