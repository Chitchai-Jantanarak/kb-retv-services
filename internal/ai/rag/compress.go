package rag

import (
	"strings"
)

type Compressor struct {
	MaxRunes int
}

func (c Compressor) Compress(candidates []Candidate) string {
	maxRunes := c.MaxRunes
	if maxRunes <= 0 {
		maxRunes = 4000
	}

	var builder strings.Builder
	for _, candidate := range candidates {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(candidate.Title)
		if candidate.Content != "" {
			builder.WriteString("\n")
			builder.WriteString(candidate.Content)
		}
		if len([]rune(builder.String())) >= maxRunes {
			break
		}
	}
	return limitRunes(builder.String(), maxRunes)
}

func limitRunes(text string, maxRunes int) string {
	runes := []rune(text)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}
