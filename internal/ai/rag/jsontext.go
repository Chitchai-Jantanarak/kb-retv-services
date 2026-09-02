package rag

import "strings"

// extractJSONObject returns the first balanced JSON object in text.
//
// The three stages that ask a provider for JSON call GenerateJSON, which sets
// the provider's structured-output mode. That mode is a request, not a
// guarantee: a model may still answer with a preamble ("Here is the JSON
// requested:") or wrap the object in a markdown fence, and a strict unmarshal
// of the whole response then fails on the first character. The failure is
// total — the reply request returns an error — for a response whose JSON is
// perfectly well formed a few characters further in.
//
// Scanning for the first balanced object is deliberately narrow. It does not
// repair malformed JSON and it does not guess: if no balanced object is
// present the original text is returned unchanged, so the caller's unmarshal
// still fails and still reports what it received.
func extractJSONObject(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		return trimmed
	}

	start := strings.IndexByte(trimmed, '{')
	if start < 0 {
		return text
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(trimmed); i++ {
		c := trimmed[i]
		switch {
		case escaped:
			escaped = false
		case c == '\\' && inString:
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
			// Braces inside a string literal are not structural.
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return trimmed[start : i+1]
			}
		}
	}
	return text
}
