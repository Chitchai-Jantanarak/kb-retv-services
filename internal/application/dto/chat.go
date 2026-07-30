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
	chatMaxAttachments     = 5
)

type ChatMessage struct {
	Role        string          `json:"role"`
	Content     string          `json:"content"`
	Attachments []AttachmentRef `json:"attachments,omitempty"`
}

type ChatRequest struct {
	Messages []ChatMessage `json:"messages"`
	Locale   string        `json:"locale"`
	Debug    bool          `json:"debug,omitempty"`
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
		if len(m.Attachments) > chatMaxAttachments {
			return apperr.New(apperr.CodeInvalidInput, "too many attachments")
		}
		if m.Content == "" && (m.Role != ChatRoleUser || len(m.Attachments) == 0) {
			return apperr.New(apperr.CodeInvalidInput, "content is required")
		}
		if err := validateAttachmentRefs(m.Attachments); err != nil {
			return err
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

const (
	ChatStatusAnswered           = "answered"
	ChatStatusOffDomain          = "off_domain"
	ChatStatusHandoff            = "handoff"
	ChatStatusNeedsClarification = "needs_clarification"
	ChatStatusNoResults          = "no_results"
	ChatStatusToolFailed         = "tool_failed"
)

type ChatSource struct {
	ID      string  `json:"id"`
	Title   string  `json:"title,omitempty"`
	Snippet string  `json:"snippet,omitempty"`
	Source  string  `json:"source,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

type ChatSearchRequest struct {
	Query   string `json:"query,omitempty"`
	Product string `json:"product,omitempty"`
	Status  string `json:"status,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

func (r ChatSearchRequest) IsZero() bool {
	return strings.TrimSpace(r.Query) == "" &&
		strings.TrimSpace(r.Product) == "" &&
		strings.TrimSpace(r.Status) == ""
}

type ChatCaseResult struct {
	ID     int64  `json:"id"`
	Code   string `json:"code,omitempty"`
	Title  string `json:"title"`
	Status string `json:"status,omitempty"`
}

type ChatActivity struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type ChatResponse struct {
	Reply          string           `json:"reply"`
	Status         string           `json:"status"`
	Sources        []ChatSource     `json:"sources"`
	Activity       []ChatActivity   `json:"activity,omitempty"`
	Case           *ChatCaseDraft   `json:"case,omitempty"`
	SearchResults  []ChatCaseResult `json:"search_results,omitempty"`
	StageTimingsMS map[string]int64 `json:"stage_timings_ms,omitempty"`
	Transcript     string           `json:"transcript,omitempty"`
}

const (
	ChatStreamEventDelta      = "delta"
	ChatStreamEventDone       = "done"
	ChatStreamEventError      = "error"
	ChatStreamEventTranscript = "transcript"
)

type ChatStreamEvent struct {
	Type          string           `json:"-"`
	Text          string           `json:"text,omitempty"`
	Status        string           `json:"status,omitempty"`
	Sources       []ChatSource     `json:"sources,omitempty"`
	Activity      []ChatActivity   `json:"activity,omitempty"`
	SearchResults []ChatCaseResult `json:"search_results,omitempty"`
	Transcript    string           `json:"transcript,omitempty"`
	Code          string           `json:"code,omitempty"`
	Message       string           `json:"message,omitempty"`
}
