package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/shared/ctxkey"
	"github.com/my/app/internal/shared/debugtrace"
	apperr "github.com/my/app/internal/shared/errors"
)

const chatDebugPermission = "ai:chat:debug"

var sensitiveToolParamParts = []string{
	"auth",
	"credential",
	"key",
	"password",
	"secret",
	"token",
}

func authorizeChatDebug(ctxPrincipal ctxkey.Principal, requested bool) bool {
	if !requested {
		return false
	}
	role := strings.ToLower(strings.TrimSpace(ctxPrincipal.Role))
	if role == "super_admin" || role == "superadmin" || role == "system" {
		return true
	}
	for _, permission := range ctxPrincipal.Perms {
		if strings.EqualFold(strings.TrimSpace(permission), chatDebugPermission) {
			return true
		}
	}
	return false
}

func buildToolDebug(resp skeleton.Response, err error, durationMS int64, traceEvents []debugtrace.Event) *dto.ChatToolDebug {
	outcome := "no_match"
	switch {
	case err == nil && resp.Called:
		outcome = "completed"
	case errors.Is(err, skeleton.ErrNeedsParam):
		outcome = "needs_param"
	case err != nil && resp.Called:
		outcome = "failed"
	case err != nil:
		outcome = "blocked"
	case resp.Matched:
		outcome = "selected"
	}

	errorCode, safeError := safeToolDebugError(err)

	return &dto.ChatToolDebug{
		ToolID:             resp.ToolID,
		Handler:            resp.Handler,
		Kind:               normalizedToolKind(resp.Kind),
		RequiredPermission: resp.RequiredPermission,
		PermissionFiltered: resp.PermFiltered,
		Params:             sanitizedToolParams(resp.Params),
		Matched:            resp.Matched,
		Called:             resp.Called,
		Mutates:            strings.EqualFold(resp.Kind, "write"),
		Outcome:            outcome,
		SelectionScore:     resp.SelectionScore,
		DurationMS:         durationMS,
		RowCount:           resp.RowCount,
		ErrorCode:          errorCode,
		Error:              safeError,
		TraceEvents:        chatDebugEvents(traceEvents),
	}
}

func safeToolDebugError(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	if errors.Is(err, skeleton.ErrNeedsParam) {
		return "needs_param", "Required tool parameters are missing."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout", "The tool or its data source exceeded the request deadline."
	}
	if appError, ok := apperr.As(err); ok {
		return string(appError.Code), truncateToolParam(strings.TrimSpace(appError.Message), 180)
	}
	message := strings.TrimSpace(err.Error())
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "not found"):
		return "not_found", truncateToolParam(message, 180)
	case strings.Contains(lower, "coverage must not be empty"), strings.Contains(lower, "company scope"):
		return "scope_rejected", "The repository rejected an empty company scope."
	}
	return "tool_failed", "Tool execution failed before a safe response was returned."
}

func chatDebugEvents(events []debugtrace.Event) []dto.ChatDebugEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]dto.ChatDebugEvent, 0, len(events))
	for _, event := range events {
		out = append(out, dto.ChatDebugEvent{
			Stage:      event.Stage,
			State:      event.State,
			Label:      event.Label,
			Context:    event.Context,
			DurationMS: event.DurationMS,
			ErrorCode:  event.ErrorCode,
			Error:      event.Error,
		})
	}
	return out
}

func normalizedToolKind(kind string) string {
	if strings.EqualFold(strings.TrimSpace(kind), "write") {
		return "write"
	}
	return "read"
}

func sanitizedToolParams(params map[string]string) map[string]string {
	if len(params) == 0 {
		return nil
	}
	sanitized := make(map[string]string, len(params))
	for key, value := range params {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if isSensitiveToolParam(normalizedKey) {
			sanitized[key] = "[redacted]"
			continue
		}
		sanitized[key] = truncateToolParam(strings.TrimSpace(value), 160)
	}
	return sanitized
}

func isSensitiveToolParam(key string) bool {
	for _, part := range sensitiveToolParamParts {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func truncateToolParam(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "…"
}

func attachChatDebug(resp dto.ChatResponse, tool *dto.ChatToolDebug, timings map[string]int64, cacheHit bool) dto.ChatResponse {
	if timings == nil {
		return resp
	}
	resp.Debug = &dto.ChatDebug{
		Tool:           tool,
		Events:         buildDebugEvents(tool, timings, cacheHit, resp.Status),
		StageTimingsMS: timings,
		CacheHit:       cacheHit,
	}
	return resp
}

func buildDebugEvents(tool *dto.ChatToolDebug, timings map[string]int64, cacheHit bool, status string) []dto.ChatDebugEvent {
	events := make([]dto.ChatDebugEvent, 0, len(timings)+6)
	events = append(events,
		dto.ChatDebugEvent{
			Stage:   "permission.debug",
			State:   "granted",
			Label:   "authorize verbose debug",
			Context: map[string]string{"required": chatDebugPermission},
		},
		dto.ChatDebugEvent{
			Stage:   "guard.company_scope",
			State:   "passed",
			Label:   "verify company scope",
			Context: map[string]string{"source": "verified service JWT", "company_id": "[present]"},
		},
	)
	if cacheHit {
		events = append(events, dto.ChatDebugEvent{Stage: "cache", State: "hit", Label: "reuse cached response"})
	}
	if tool != nil {
		selectionContext := map[string]string{
			"tool_id": tool.ToolID,
			"score":   fmt.Sprintf("%.3f", tool.SelectionScore),
		}
		selectionState := "no_match"
		if tool.Matched {
			selectionState = "selected"
		} else if tool.PermissionFiltered {
			selectionState = "permission_filtered"
		}
		events = append(events, dto.ChatDebugEvent{
			Stage:   "tool.select",
			State:   selectionState,
			Label:   "select tool",
			Context: selectionContext,
		})

		permissionState := "not_selected"
		permissionContext := map[string]string{}
		if tool.RequiredPermission != "" {
			permissionContext["required"] = tool.RequiredPermission
			permissionState = "granted"
		}
		if tool.PermissionFiltered {
			permissionContext["result"] = "no permitted tool candidate"
			permissionState = "denied"
		}
		if len(permissionContext) > 0 {
			events = append(events, dto.ChatDebugEvent{
				Stage:   "tool.permission",
				State:   permissionState,
				Label:   "check tool permission",
				Context: permissionContext,
			})
		}

		if tool.Called || tool.Matched {
			callContext := make(map[string]string, len(tool.Params)+2)
			callContext["handler"] = tool.Handler
			callContext["operation"] = strings.ToUpper(tool.Kind)
			for key, value := range tool.Params {
				callContext[key] = value
			}
			events = append(events, dto.ChatDebugEvent{
				Stage:      "tool.call",
				State:      tool.Outcome,
				Label:      "call " + tool.ToolID,
				Context:    callContext,
				DurationMS: tool.DurationMS,
				ErrorCode:  tool.ErrorCode,
				Error:      tool.Error,
			})
		}

		events = append(events, tool.TraceEvents...)

		if tool.Called {
			events = append(events, dto.ChatDebugEvent{
				Stage: "tool.result",
				State: tool.Outcome,
				Label: "tool result",
				Context: map[string]string{
					"rows": fmt.Sprintf("%d", tool.RowCount),
				},
				DurationMS: tool.DurationMS,
			})
		}
	}

	stageOrder := []string{"router", "tool", "cache", "knowledge", "profile", "render", "resolve", "generate", "parse", "search", "store_cache"}
	for _, stage := range stageOrder {
		duration, ok := timings[stage]
		if !ok {
			continue
		}
		events = append(events, dto.ChatDebugEvent{
			Stage:      "pipeline." + stage,
			State:      "completed",
			Label:      strings.ReplaceAll(stage, "_", " "),
			DurationMS: duration,
		})
	}
	return events
}
