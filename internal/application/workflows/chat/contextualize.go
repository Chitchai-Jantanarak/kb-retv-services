package chat

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/tools"
	"github.com/my/app/internal/shared/debugtrace"
)

const (
	contextualizeMaxTurns = 6
	contextualizeMaxChars = 400
)

var contextualizeCodePattern = regexp.MustCompile(`(?i)\b(?:rep|draft)[-:\s]?\d+\b`)
var contextualizeDigitPattern = regexp.MustCompile(`\d+`)

// contextualize rewrites the last user message into a standalone request when
// the conversation has more than one user turn. Returns the text to route on
// and a state for the debug trace: skipped | ok | llm_failed | rejected.
func (w *Workflow) contextualize(ctx context.Context, locale string, req dto.ChatRequest, raw string, candidates []string, companyID int64) (string, string) {
	userTurns := 0
	for _, m := range req.Messages {
		if m.Role == dto.ChatRoleUser {
			userTurns++
		}
	}
	if userTurns < 2 {
		return raw, "skipped"
	}

	start := time.Now()
	transcript := contextualizeTranscript(req.Messages)

	output, ok := w.smallModelCall(ctx, w.contextualizeTmpl, map[string]string{
		"language":   promptLanguage(locale),
		"question":   raw,
		"transcript": transcript,
	}, companyID)
	if !ok {
		emitContextualizeEvent(ctx, "llm_failed", "", start)
		return raw, "llm_failed"
	}

	rewritten, valid := validateContextualizeOutput(output, req, candidates)
	if !valid {
		emitContextualizeEvent(ctx, "rejected", "", start)
		return raw, "rejected"
	}

	emitContextualizeEvent(ctx, "ok", rewritten, start)
	return rewritten, "ok"
}

// contextualizeTranscript renders the transcript window fed to the rewrite
// call: the last contextualizeMaxTurns messages before the final user turn,
// user content escaped and assistant content sanitized the same way the
// generative path treats untrusted transcript data.
func contextualizeTranscript(messages []dto.ChatMessage) string {
	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == dto.ChatRoleUser {
			lastUserIdx = i
			break
		}
	}
	subset := messages
	if lastUserIdx >= 0 {
		subset = messages[:lastUserIdx]
	}
	if len(subset) > contextualizeMaxTurns {
		subset = subset[len(subset)-contextualizeMaxTurns:]
	}

	var transcript strings.Builder
	for _, m := range subset {
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		if m.Role == dto.ChatRoleUser {
			transcript.WriteString("User: " + escapeFence(m.Content) + "\n")
		} else {
			transcript.WriteString("Assistant: " + sanitizeUntrusted(m.Content) + "\n")
		}
	}
	return transcript.String()
}

func validateContextualizeOutput(output string, req dto.ChatRequest, candidates []string) (string, bool) {
	line := stripSurroundingQuotes(firstNonEmptyLine(output))
	if line == "" {
		return "", false
	}
	if len([]rune(line)) > contextualizeMaxChars {
		return "", false
	}
	lower := strings.ToLower(line)
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") {
		return "", false
	}

	codes, digits := contextualizeAllowed(req, candidates)
	for _, match := range contextualizeCodePattern.FindAllString(line, -1) {
		ref, ok := tools.ParseCaseRef(match)
		if !ok || !codes[ref.Code] {
			return "", false
		}
	}
	for _, match := range contextualizeDigitPattern.FindAllString(line, -1) {
		if !digits[match] {
			return "", false
		}
	}

	return line, true
}

// contextualizeAllowed builds the set of case codes and bare digit runs the
// rewrite is permitted to resolve to: identifiers the user themself typed in
// any of their own turns, plus identifiers the server returned as cite
// candidates. Identifiers that appear only in assistant text are excluded.
func contextualizeAllowed(req dto.ChatRequest, candidates []string) (codes, digits map[string]bool) {
	codes = make(map[string]bool)
	digits = make(map[string]bool)

	addFrom := func(s string) {
		for _, match := range contextualizeCodePattern.FindAllString(s, -1) {
			if ref, ok := tools.ParseCaseRef(match); ok {
				codes[ref.Code] = true
			}
		}
		for _, match := range contextualizeDigitPattern.FindAllString(s, -1) {
			digits[match] = true
		}
	}

	for _, m := range req.Messages {
		if m.Role == dto.ChatRoleUser {
			addFrom(m.Content)
		}
	}
	for _, c := range candidates {
		addFrom(c)
	}
	return codes, digits
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

var contextualizeQuotePairs = map[rune]rune{'"': '"', '\'': '\'', '“': '”'}

func stripSurroundingQuotes(s string) string {
	r := []rune(s)
	if len(r) < 2 {
		return s
	}
	closing, ok := contextualizeQuotePairs[r[0]]
	if !ok || r[len(r)-1] != closing {
		return s
	}
	return strings.TrimSpace(string(r[1 : len(r)-1]))
}

func emitContextualizeEvent(ctx context.Context, state, rewritten string, start time.Time) {
	if !debugtrace.Enabled(ctx) {
		return
	}
	debugtrace.Add(ctx, debugtrace.Event{
		Stage:      "contextualize",
		State:      state,
		Label:      "contextualize follow-up",
		DurationMS: time.Since(start).Milliseconds(),
		Context:    map[string]string{"rewritten": rewritten},
	})
}
