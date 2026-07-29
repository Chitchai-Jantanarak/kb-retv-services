package chat

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/workflows/intake"
)

var replyRe = regexp.MustCompile(`"reply"\s*:\s*"((?:[^"\\]|\\.)*)`)

var caseCodeRe = regexp.MustCompile(`(?i)\b(REP-\d+)\b`)

func searchQueryText(raw string) string {
	if m := caseCodeRe.FindString(raw); m != "" {
		return strings.ToUpper(m)
	}
	if len(strings.Fields(raw)) > searchQueryMaxWords {
		return ""
	}
	return raw
}

type llmTurn struct {
	Reply  string                 `json:"reply"`
	Case   *dto.ChatCaseDraft     `json:"case"`
	Search *dto.ChatSearchRequest `json:"search"`
}

func parseTurn(raw string) (llmTurn, bool) {
	txt := strings.TrimSpace(raw)
	txt = strings.TrimPrefix(txt, "```json")
	txt = strings.TrimPrefix(txt, "```")
	txt = strings.TrimSuffix(txt, "```")
	txt = strings.TrimSpace(txt)

	var turn llmTurn
	if err := json.Unmarshal([]byte(txt), &turn); err == nil && strings.TrimSpace(turn.Reply) != "" {
		turn.Reply = strings.TrimSpace(turn.Reply)
		return turn, true
	}
	if start, end := strings.Index(txt, "{"), strings.LastIndex(txt, "}"); start >= 0 && end > start {
		if err := json.Unmarshal([]byte(txt[start:end+1]), &turn); err == nil && strings.TrimSpace(turn.Reply) != "" {
			turn.Reply = strings.TrimSpace(turn.Reply)
			return turn, true
		}
	}
	if m := replyRe.FindStringSubmatch(txt); len(m) == 2 {
		var reply string
		if err := json.Unmarshal([]byte(`"`+m[1]+`"`), &reply); err == nil && strings.TrimSpace(reply) != "" {
			return llmTurn{Reply: strings.TrimSpace(reply)}, false
		}
	}
	return llmTurn{Reply: strings.TrimSpace(raw)}, false
}

func normalizeSearch(req *dto.ChatSearchRequest) *dto.ChatSearchRequest {
	if req == nil {
		return nil
	}
	req.Query = strings.TrimSpace(req.Query)
	req.Product = strings.TrimSpace(req.Product)
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if req.IsZero() {
		return nil
	}
	req.Query = searchQueryText(req.Query)
	if req.Limit <= 0 || req.Limit > chatSearchMaxLimit {
		req.Limit = chatSearchLimit
	}
	return req
}

func normalizeCaseDraft(draft *dto.ChatCaseDraft) *dto.ChatCaseDraft {
	if draft == nil {
		return nil
	}
	draft.Problem = strings.TrimSpace(draft.Problem)
	draft.Detail = strings.TrimSpace(draft.Detail)
	draft.Product = strings.TrimSpace(draft.Product)
	if draft.Problem == "" && draft.Detail == "" && draft.Product == "" {
		return nil
	}
	if draft.Ready && !caseComplete(*draft) {
		draft.Ready = false
	}
	return draft
}

func caseComplete(draft dto.ChatCaseDraft) bool {
	if draft.Problem == "" {
		return false
	}
	return len(intake.DefaultSpec().Missing(map[string]string{
		intake.FieldProblemDetail: draft.Detail,
		intake.FieldProduct:       draft.Product,
		intake.FieldSeverity:      draft.Severity,
		intake.FieldContact:       draft.Contact,
	})) == 0
}
