package chat

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/my/app/internal/application/dto"
)

var replyRe = regexp.MustCompile(`"reply"\s*:\s*"((?:[^"\\]|\\.)*)`)

type llmTurn struct {
	Reply string             `json:"reply"`
	Case  *dto.ChatCaseDraft `json:"case"`
}

func parseTurn(raw string) llmTurn {
	txt := strings.TrimSpace(raw)
	txt = strings.TrimPrefix(txt, "```json")
	txt = strings.TrimPrefix(txt, "```")
	txt = strings.TrimSuffix(txt, "```")
	txt = strings.TrimSpace(txt)

	var turn llmTurn
	if err := json.Unmarshal([]byte(txt), &turn); err == nil && strings.TrimSpace(turn.Reply) != "" {
		turn.Reply = strings.TrimSpace(turn.Reply)
		return turn
	}
	if start, end := strings.Index(txt, "{"), strings.LastIndex(txt, "}"); start >= 0 && end > start {
		if err := json.Unmarshal([]byte(txt[start:end+1]), &turn); err == nil && strings.TrimSpace(turn.Reply) != "" {
			turn.Reply = strings.TrimSpace(turn.Reply)
			return turn
		}
	}
	if m := replyRe.FindStringSubmatch(txt); len(m) == 2 {
		var reply string
		if err := json.Unmarshal([]byte(`"`+m[1]+`"`), &reply); err == nil && strings.TrimSpace(reply) != "" {
			return llmTurn{Reply: strings.TrimSpace(reply)}
		}
	}
	return llmTurn{Reply: strings.TrimSpace(raw)}
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
	if draft.Ready && (draft.Problem == "" || draft.Detail == "" || draft.Product == "") {
		draft.Ready = false
	}
	return draft
}
