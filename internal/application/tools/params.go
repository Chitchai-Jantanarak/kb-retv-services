package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/my/app/internal/shared/ctxkey"
)

type NameSource interface {
	Names(ctx context.Context, lookup string) ([]string, error)
}

type compiledParam struct {
	name     string
	required bool
	re       *regexp.Regexp
	keywords map[string]string
	lookup   string
	pattern  string
}

func compileParams(params []Param) ([]compiledParam, error) {
	out := make([]compiledParam, 0, len(params))
	for _, p := range params {
		cp := compiledParam{name: p.Name, required: p.Required, lookup: p.Lookup, pattern: p.Pattern}
		if p.Pattern != "" {
			re, err := regexp.Compile(p.Pattern)
			if err != nil {
				return nil, fmt.Errorf("tools: param %q pattern: %w", p.Name, err)
			}
			cp.re = re
		}
		if len(p.Map) > 0 {
			cp.keywords = make(map[string]string, len(p.Map))
			for k, v := range p.Map {
				cp.keywords[strings.ToLower(k)] = v
			}
		}
		out = append(out, cp)
	}
	return out, nil
}

func (s *Selector) extractParams(ctx context.Context, params []compiledParam, text string) (map[string]string, bool, error) {
	if len(params) == 0 {
		return nil, true, nil
	}
	lower := strings.ToLower(text)
	out := make(map[string]string, len(params))
	satisfied := true
	for _, p := range params {
		value := ""
		if p.re != nil {
			if isCaseRefParamPattern(p.pattern) {
				if ref, ok := ParseCaseRef(text); ok {
					value = ref.Code
				}
			} else {
				value = strings.ToUpper(strings.TrimSpace(p.re.FindString(text)))
			}
		}
		if value == "" && p.keywords != nil {
			bestLen := 0
			for keyword, mapped := range p.keywords {
				if strings.Contains(lower, keyword) && len(keyword) > bestLen {
					value, bestLen = mapped, len(keyword)
				}
			}
		}
		if value == "" && p.lookup != "" && s.names != nil {
			names, err := s.names.Names(ctx, p.lookup)
			if err != nil {
				return nil, false, fmt.Errorf("tools: lookup %s: %w", p.lookup, err)
			}
			value = matchLookupName(names, lower)
		}
		if value == "" && p.re != nil {
			if candidates := matchingCandidates(ctxkey.CiteCandidates(ctx), p.re); len(candidates) > 0 {
				if resolved, ok := ResolveReference(text, candidates); ok {
					value = strings.ToUpper(resolved)
				}
			}
		}
		if value != "" {
			out[p.name] = value
		} else if p.required {
			satisfied = false
		}
	}
	return out, satisfied, nil
}

const minLookupNameRunes = 3

func isSegmentedScript(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func containsNameAtWordBoundary(lowerText, ln string) bool {
	if !isSegmentedScript(ln) {
		return strings.Contains(lowerText, ln)
	}
	runes := []rune(lowerText)
	target := []rune(ln)
	for i := 0; i+len(target) <= len(runes); i++ {
		if string(runes[i:i+len(target)]) != ln {
			continue
		}
		if i > 0 && isWordRune(runes[i-1]) {
			continue
		}
		if end := i + len(target); end < len(runes) && isWordRune(runes[end]) {
			continue
		}
		return true
	}
	return false
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func matchLookupName(names []string, lowerText string) string {
	best := ""
	for _, n := range names {
		ln := strings.ToLower(strings.TrimSpace(n))
		if ln == "" || len([]rune(ln)) < minLookupNameRunes {
			continue
		}
		if containsNameAtWordBoundary(lowerText, ln) && len(ln) > len(best) {
			best = strings.TrimSpace(n)
		}
	}
	if best != "" {
		return best
	}

	bestTokens, tied := 0, false
	for _, n := range names {
		tokens := strings.Fields(strings.ToLower(n))
		if len(tokens) < 2 {
			continue
		}
		longest := ""
		for _, tok := range tokens {
			if len([]rune(tok)) > len([]rune(longest)) {
				longest = tok
			}
		}
		matched, longestMatched := 0, false
		for _, tok := range tokens {
			if len([]rune(tok)) < minLookupNameRunes || !containsNameAtWordBoundary(lowerText, tok) {
				continue
			}
			matched++
			if tok == longest {
				longestMatched = true
			}
		}
		if !longestMatched {
			continue
		}
		if matched > bestTokens {
			bestTokens, tied, best = matched, false, strings.TrimSpace(n)
		} else if matched == bestTokens && strings.TrimSpace(n) != best {
			tied = true
		}
	}
	if tied {
		return ""
	}
	return best
}

func requiredCount(params []compiledParam) int {
	n := 0
	for _, p := range params {
		if p.required {
			n++
		}
	}
	return n
}

func missingRequired(params []compiledParam, extracted map[string]string) []string {
	var missing []string
	for _, p := range params {
		if p.required && extracted[p.name] == "" {
			missing = append(missing, p.name)
		}
	}
	return missing
}

func entityMatched(params []compiledParam, extracted map[string]string) int {
	n := 0
	for _, p := range params {
		if p.required && extracted[p.name] != "" && (p.lookup != "" || isCaseRefParamPattern(p.pattern)) {
			n++
		}
	}
	return n
}

func requiredMatched(params []compiledParam, extracted map[string]string) int {
	n := 0
	for _, p := range params {
		if p.required && extracted[p.name] != "" {
			n++
		}
	}
	return n
}
