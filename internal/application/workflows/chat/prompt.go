package chat

import (
	"fmt"
	"strings"

	"github.com/my/app/internal/application/dto"
)

const (
	knowledgeChunkLimit      = 3
	knowledgeSnippetMaxChars = 600
	chatSearchLimit          = 5
)

func promptLanguage(locale string) string {
	if locale == dto.ChatLocaleEnglish {
		return "English"
	}
	return "Thai"
}

func knowledgeSection(block string) string {
	if block == "" {
		return ""
	}
	return "\n\nReference notes from the knowledge base (may be partial):\n" + block
}

func profileSection(block string) string {
	if strings.TrimSpace(block) == "" {
		return ""
	}
	return "\n\nCompany profile (use to answer identity/product questions):\n" + block
}

func buildTranscript(messages []dto.ChatMessage) string {
	var transcript strings.Builder
	for _, m := range messages {
		if m.Role == dto.ChatRoleUser {
			fmt.Fprintf(&transcript, "User: %s\n", m.Content)
		} else {
			fmt.Fprintf(&transcript, "Assistant: %s\n", m.Content)
		}
	}
	return transcript.String()
}
