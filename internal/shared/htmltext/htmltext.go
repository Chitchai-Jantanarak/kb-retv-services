package htmltext

import (
	"regexp"
	"strings"
)

var (
	imgTagRe  = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	dataURLRe = regexp.MustCompile(`(?is)data:[a-z0-9.+-]+/[a-z0-9.+-]+;base64,[A-Za-z0-9+/=]+`)
)

func StripInlineImages(s string) string {
	if s == "" {
		return s
	}
	s = imgTagRe.ReplaceAllString(s, "")
	s = dataURLRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}
