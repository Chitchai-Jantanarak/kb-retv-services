package chat

import (
	"strings"

	"github.com/my/app/internal/application/dto"
)

const transcriptMaxTurns = 6

// transcriptWindow renders the transcript window handed to the tool selector:
// the last transcriptMaxTurns messages before the final user turn, user content
// escaped and assistant content sanitized the same way the generative path
// treats untrusted transcript data.
func transcriptWindow(messages []dto.ChatMessage) string {
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
	if len(subset) > transcriptMaxTurns {
		subset = subset[len(subset)-transcriptMaxTurns:]
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
