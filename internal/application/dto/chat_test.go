package dto

import (
	"strings"
	"testing"

	apperr "github.com/my/app/internal/shared/errors"
)

func TestChatRequestValidateNormalizesRolesAndLocale(t *testing.T) {
	req := ChatRequest{
		Messages: []ChatMessage{
			{Role: " USER ", Content: " printer offline "},
			{Role: "Assistant", Content: " ok "},
		},
		Locale: "fr",
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if req.Messages[0].Role != ChatRoleUser {
		t.Fatalf("Role = %q, want %q", req.Messages[0].Role, ChatRoleUser)
	}
	if req.Messages[0].Content != "printer offline" {
		t.Fatalf("Content = %q, want printer offline", req.Messages[0].Content)
	}
	if req.Locale != ChatLocaleThai {
		t.Fatalf("Locale = %q, want %q", req.Locale, ChatLocaleThai)
	}
}

func TestChatRequestValidateKeepsEnglishLocale(t *testing.T) {
	req := ChatRequest{
		Messages: []ChatMessage{{Role: ChatRoleUser, Content: "hi"}},
		Locale:   ChatLocaleEnglish,
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if req.Locale != ChatLocaleEnglish {
		t.Fatalf("Locale = %q, want %q", req.Locale, ChatLocaleEnglish)
	}
}

func TestChatRequestValidateTruncatesToRecentMessages(t *testing.T) {
	req := ChatRequest{}
	for i := 0; i < chatMaxMessages+3; i++ {
		req.Messages = append(req.Messages, ChatMessage{Role: ChatRoleUser, Content: "m"})
	}
	req.Messages[len(req.Messages)-1].Content = "latest"

	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(req.Messages) != chatMaxMessages {
		t.Fatalf("len(Messages) = %d, want %d", len(req.Messages), chatMaxMessages)
	}
	if req.Messages[len(req.Messages)-1].Content != "latest" {
		t.Fatalf("last message = %q, want latest", req.Messages[len(req.Messages)-1].Content)
	}
}

func TestChatRequestValidateRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		req  ChatRequest
	}{
		{name: "empty messages", req: ChatRequest{}},
		{name: "unknown role", req: ChatRequest{Messages: []ChatMessage{{Role: "system", Content: "x"}}}},
		{name: "blank content", req: ChatRequest{Messages: []ChatMessage{{Role: ChatRoleUser, Content: "  "}}}},
		{name: "oversize transcript", req: ChatRequest{Messages: []ChatMessage{{
			Role:    ChatRoleUser,
			Content: strings.Repeat("a", chatMaxTranscriptChars+1),
		}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if err == nil {
				t.Fatal("Validate() error = nil, want invalid_input")
			}
			appErr, ok := apperr.As(err)
			if !ok || appErr.Code != apperr.CodeInvalidInput {
				t.Fatalf("Validate() error = %v, want code %s", err, apperr.CodeInvalidInput)
			}
		})
	}
}

func TestChatRequestValidateAllowsAttachmentOnlyUserMessage(t *testing.T) {
	req := ChatRequest{
		Messages: []ChatMessage{
			{Role: ChatRoleUser, Attachments: []AttachmentRef{{StorageKey: "s3://bucket/key.png"}}},
		},
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil for attachment-only user message", err)
	}
}

func TestChatRequestValidateRejectsEmptyAssistantContent(t *testing.T) {
	req := ChatRequest{
		Messages: []ChatMessage{
			{Role: ChatRoleUser, Content: "hi"},
			{Role: ChatRoleAssistant, Attachments: []AttachmentRef{{StorageKey: "s3://bucket/key.png"}}},
		},
	}
	err := req.Validate()
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeInvalidInput {
		t.Fatalf("Validate() error = %v, want invalid_input for empty assistant content", err)
	}
}

func TestChatRequestValidateRejectsTooManyAttachments(t *testing.T) {
	attachments := make([]AttachmentRef, 6)
	for i := range attachments {
		attachments[i] = AttachmentRef{StorageKey: "s3://bucket/key.png"}
	}
	req := ChatRequest{
		Messages: []ChatMessage{
			{Role: ChatRoleUser, Attachments: attachments},
		},
	}
	err := req.Validate()
	if err == nil || !strings.Contains(err.Error(), "too many attachments") {
		t.Fatalf("Validate() error = %v, want too many attachments", err)
	}
}

func TestChatRequestValidatePropagatesInvalidAttachment(t *testing.T) {
	req := ChatRequest{
		Messages: []ChatMessage{
			{Role: ChatRoleUser, Attachments: []AttachmentRef{{URL: "data:image/png;base64,AAAA"}}},
		},
	}
	err := req.Validate()
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != apperr.CodeInvalidInput {
		t.Fatalf("Validate() error = %v, want invalid_input for bad attachment url", err)
	}
	if !strings.Contains(err.Error(), "stored media") {
		t.Fatalf("Validate() error = %v, want attachment url error propagated", err)
	}
}

func TestChatRequestLastUserMessage(t *testing.T) {
	req := ChatRequest{Messages: []ChatMessage{
		{Role: ChatRoleUser, Content: "first"},
		{Role: ChatRoleUser, Content: "second"},
		{Role: ChatRoleAssistant, Content: "reply"},
	}}

	if got := req.LastUserMessage(); got != "second" {
		t.Fatalf("LastUserMessage() = %q, want second", got)
	}

	empty := ChatRequest{Messages: []ChatMessage{{Role: ChatRoleAssistant, Content: "reply"}}}
	if got := empty.LastUserMessage(); got != "" {
		t.Fatalf("LastUserMessage() = %q, want empty", got)
	}
}
