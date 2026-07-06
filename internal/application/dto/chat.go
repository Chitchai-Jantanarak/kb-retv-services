package dto

import (
	"strings"

	apperr "github.com/my/app/internal/shared/errors"
)

const (
	ChatRoleUser      = "user"
	ChatRoleAssistant = "assistant"

	ChatLocaleThai    = "th"
	ChatLocaleEnglish = "en"
)

const (
	chatMaxMessages        = 12
	chatMaxTranscriptChars = 16000
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
	Locale   string        `json:"locale"`
}

func (r *ChatRequest) Normalize() {
	if len(r.Messages) > chatMaxMessages {
		r.Messages = r.Messages[len(r.Messages)-chatMaxMessages:]
	}
	for i := range r.Messages {
		r.Messages[i].Role = strings.ToLower(normalizeText(r.Messages[i].Role))
		r.Messages[i].Content = normalizeText(r.Messages[i].Content)
	}
	if r.Locale != ChatLocaleEnglish {
		r.Locale = ChatLocaleThai
	}
}

func (r *ChatRequest) Validate() error {
	r.Normalize()
	if len(r.Messages) == 0 {
		return apperr.New(apperr.CodeInvalidInput, "messages is required")
	}
	total := 0
	for _, m := range r.Messages {
		if m.Role != ChatRoleUser && m.Role != ChatRoleAssistant {
			return apperr.New(apperr.CodeInvalidInput, "role must be user or assistant")
		}
		if m.Content == "" {
			return apperr.New(apperr.CodeInvalidInput, "content is required")
		}
		total += len(m.Content)
	}
	if total > chatMaxTranscriptChars {
		return apperr.New(apperr.CodeInvalidInput, "conversation too large")
	}
	return nil
}

func (r ChatRequest) LastUserMessage() string {
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if r.Messages[i].Role == ChatRoleUser {
			return r.Messages[i].Content
		}
	}
	return ""
}

type ChatCaseDraft struct {
	Problem  string `json:"problem"`
	Detail   string `json:"detail"`
	Product  string `json:"product"`
	Severity string `json:"severity,omitempty"`
	Contact  string `json:"contact,omitempty"`
	Ready    bool   `json:"ready"`
}

type ChatResponse struct {
	Reply   string         `json:"reply"`
	Sources []string       `json:"sources"`
	Case    *ChatCaseDraft `json:"case,omitempty"`
}
