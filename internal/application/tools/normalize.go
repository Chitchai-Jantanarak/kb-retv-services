package tools

import (
	"regexp"
	"strings"
	"unicode"
)

var loanwords = map[string]string{
	"report":    "รายงาน",
	"reports":   "รายงาน",
	"draft":     "ฉบับร่าง",
	"drafts":    "ฉบับร่าง",
	"case":      "เคส",
	"cases":     "เคส",
	"ticket":    "เคส",
	"tickets":   "เคส",
	"status":    "สถานะ",
	"product":   "สินค้า",
	"products":  "สินค้า",
	"customer":  "ลูกค้า",
	"customers": "ลูกค้า",
	"employee":  "พนักงาน",
	"employees": "พนักงาน",
	"email":     "อีเมล",
	"emails":    "อีเมล",
}

var asciiWord = regexp.MustCompile(`[A-Za-z]+`)

func hasThai(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Thai, r) {
			return true
		}
	}
	return false
}

func normalizeQuery(text string) string {
	if !hasThai(text) {
		return text
	}
	return asciiWord.ReplaceAllStringFunc(text, func(w string) string {
		if th, ok := loanwords[strings.ToLower(w)]; ok {
			return th
		}
		return w
	})
}
