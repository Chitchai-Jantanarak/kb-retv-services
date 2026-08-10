package tools

import (
	"regexp"
	"strconv"
)

type CaseRefNamespace string

const (
	CaseRefRep   CaseRefNamespace = "rep"
	CaseRefDraft CaseRefNamespace = "draft"
)

type CaseRef struct {
	Namespace CaseRefNamespace
	ID        int64
	Code      string
}

var (
	repRefRe       = regexp.MustCompile(`(?i)\brep[-:\s]?(\d+)\b`)
	draftRefRe     = regexp.MustCompile(`(?i)\bdraft[-:\s]?(\d+)\b`)
	caseRefStripRe = regexp.MustCompile(`(?i)\b(?:rep|draft)[-:\s]?\d+\b`)
)

const caseCodeParamPattern = `(?i)\bREP-?\d+\b`

func ParseCaseRef(text string) (CaseRef, bool) {
	if m := repRefRe.FindStringSubmatch(text); m != nil {
		id, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return CaseRef{}, false
		}
		return CaseRef{Namespace: CaseRefRep, ID: id, Code: "REP-" + m[1]}, true
	}
	if m := draftRefRe.FindStringSubmatch(text); m != nil {
		id, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return CaseRef{}, false
		}
		return CaseRef{Namespace: CaseRefDraft, ID: id, Code: "DRAFT-" + m[1]}, true
	}
	return CaseRef{}, false
}

func StripCaseRefs(text string) string {
	return caseRefStripRe.ReplaceAllString(text, "")
}

func isCaseRefParamPattern(pattern string) bool {
	return pattern == caseCodeParamPattern
}
