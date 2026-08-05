package tools

import (
	"regexp"
	"strconv"
	"strings"
)

var recencyCues = []string{"ล่าสุด", "ล่าสุดนี้", "latest", "last one", "most recent", "newest"}

var anaphoraCues = []string{"อันนี้", "ตัวนี้", "เคสนี้", "อันนั้น", "มัน", "this one", "that one", " it", "this case"}

var ordinalWords = map[string]int{
	"แรก": 1, "อันแรก": 1, "ตัวแรก": 1, "first": 1,
	"ที่สอง": 2, "อันที่สอง": 2, "second": 2,
	"ที่สาม": 3, "อันที่สาม": 3, "third": 3,
}

var ordinalDigitRe = regexp.MustCompile(`(?:ที่|อันที่|ตัวที่|number|no\.?|#)\s*(\d{1,2})|\b(\d{1,2})(?:st|nd|rd|th)\b`)

func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

func ordinalIndex(lower string) (int, bool) {
	if m := ordinalDigitRe.FindStringSubmatch(lower); m != nil {
		raw := m[1]
		if raw == "" {
			raw = m[2]
		}
		if n, err := strconv.Atoi(raw); err == nil && n >= 1 {
			return n, true
		}
	}
	for word, n := range ordinalWords {
		if strings.Contains(lower, word) {
			return n, true
		}
	}
	return 0, false
}

func ResolveReference(text string, candidates []string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return "", false
	}

	for _, c := range candidates {
		if strings.Contains(lower, strings.ToLower(c)) {
			return c, true
		}
	}
	if len(candidates) > 0 {
		if explicit := candidateShapeRe(candidates[0]); explicit != nil {
			if m := explicit.FindString(text); m != "" {
				return strings.ToUpper(m), true
			}
		}
	}

	if len(candidates) == 0 {
		return "", false
	}

	if n, ok := ordinalIndex(lower); ok {
		if n > len(candidates) {
			return "", false
		}
		return candidates[n-1], true
	}
	if containsAny(lower, recencyCues) {
		return candidates[0], true
	}
	if len(candidates) == 1 && containsAny(lower, anaphoraCues) {
		return candidates[0], true
	}
	return "", false
}

var citeShapeRe = regexp.MustCompile(`^([A-Za-z]+)[-_]?(\d+)$`)

func candidateShapeRe(sample string) *regexp.Regexp {
	m := citeShapeRe.FindStringSubmatch(strings.TrimSpace(sample))
	if m == nil {
		return nil
	}
	re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(m[1]) + `-?\d+\b`)
	if err != nil {
		return nil
	}
	return re
}

func matchingCandidates(candidates []string, re *regexp.Regexp) []string {
	if re == nil {
		return nil
	}
	out := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, c := range candidates {
		for _, m := range re.FindAllString(c, -1) {
			if _, dup := seen[m]; dup {
				continue
			}
			seen[m] = struct{}{}
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func CiteCandidatesFromTurns(assistantTurns []string, pattern string) []string {
	if pattern == "" {
		return nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	for i := len(assistantTurns) - 1; i >= 0; i-- {
		found := re.FindAllString(assistantTurns[i], -1)
		if len(found) == 0 {
			continue
		}
		seen := make(map[string]struct{}, len(found))
		out := make([]string, 0, len(found))
		for _, f := range found {
			if _, dup := seen[f]; dup {
				continue
			}
			seen[f] = struct{}{}
			out = append(out, f)
		}
		return out
	}
	return nil
}
