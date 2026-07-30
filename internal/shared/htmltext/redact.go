package htmltext

import "regexp"

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(pass(word)?|pwd|passwd)\s*[:=]\s*\S+`),
	regexp.MustCompile(`รหัสผ่าน\s*[:=]?\s*\S+`),
	regexp.MustCompile(`(?i)\b(api[_-]?key|secret|token|bearer)\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)\b(user(name)?|login|uid)\s*[:=]\s*\S+\s*/\s*\S+`),
}

func RedactSecrets(s string) string {
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, "[redacted]")
	}
	return s
}
