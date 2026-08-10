package intake

import (
	"regexp"
	"strings"
)

const (
	ReasonListUnsubscribe = "list_unsubscribe"
	ReasonAutoSubmitted   = "auto_submitted"
	ReasonSenderNoReply   = "sender_no_reply"
	ReasonPrecedenceBulk  = "precedence_bulk"
	ReasonReferencedCase  = "referenced_case"
	ReasonThreadMatched   = "thread_matched"
	ReasonHasAttachments  = "has_attachments"
	ReasonSenderKnown     = "sender_known"
	ReasonBodyThin        = "body_thin"
	ReasonLinkHeavy       = "link_heavy"
)

const (
	baseScore = 50

	weightListUnsubscribe = -40
	weightAutoSubmitted   = -35
	weightSenderNoReply   = -18
	weightPrecedenceBulk  = -6
	weightReferencedCase  = 30
	weightThreadMatched   = 30
	weightHasAttachments  = 12
	weightSenderKnown     = 12
	weightBodyThin        = -8
	weightLinkHeavy       = -8

	thinBodyChars  = 120
	linkHeavyPerKB = 4.0
	scoreFloor     = 5
	scoreCeiling   = 95
)

var (
	noReplyPattern = regexp.MustCompile(`(?i)(^|[._-])(no-?reply|do-?not-?reply)([._-]|@)`)
	linkPattern    = regexp.MustCompile(`(?i)https?://`)
)

// Signals carries the deterministic evidence Score grades on. Every field
// here must be knowable before any LLM call: headers, thread state, and
// coarse shape of the message, never model-extracted content.
type Signals struct {
	Sender          string
	Subject         string
	Body            string
	ListUnsubscribe bool
	AutoSubmitted   string
	Precedence      string
	HasAttachments  bool
	ReferencedCase  string
	ThreadMatched   bool
	SenderKnown     bool
}

// Score grades an intake on deterministic evidence only, computable before
// any LLM call. It never returns 0 or 100: absence of evidence is not
// proof, so the ends of the range stay reserved. Precedence is a weak,
// non-standard signal by design — it must never, on its own, be enough to
// flip a message from looking genuine to looking bulk.
func Score(sig Signals) (int, []string) {
	reasons := make([]string, 0, 8)
	score := baseScore

	if sig.ListUnsubscribe {
		score += weightListUnsubscribe
		reasons = append(reasons, ReasonListUnsubscribe)
	}

	if autoSubmitted := strings.ToLower(strings.TrimSpace(sig.AutoSubmitted)); autoSubmitted != "" && autoSubmitted != "no" {
		score += weightAutoSubmitted
		reasons = append(reasons, ReasonAutoSubmitted)
	}

	if noReplyPattern.MatchString(sig.Sender) {
		score += weightSenderNoReply
		reasons = append(reasons, ReasonSenderNoReply)
	}

	if isBulkPrecedence(sig.Precedence) {
		score += weightPrecedenceBulk
		reasons = append(reasons, ReasonPrecedenceBulk)
	}

	if strings.TrimSpace(sig.ReferencedCase) != "" {
		score += weightReferencedCase
		reasons = append(reasons, ReasonReferencedCase)
	}

	if sig.ThreadMatched {
		score += weightThreadMatched
		reasons = append(reasons, ReasonThreadMatched)
	}

	if sig.HasAttachments {
		score += weightHasAttachments
		reasons = append(reasons, ReasonHasAttachments)
	}

	if sig.SenderKnown {
		score += weightSenderKnown
		reasons = append(reasons, ReasonSenderKnown)
	}

	body := strings.TrimSpace(sig.Body)
	switch {
	case len(body) < thinBodyChars:
		score += weightBodyThin
		reasons = append(reasons, ReasonBodyThin)
	case linksPerKB(body) >= linkHeavyPerKB:
		score += weightLinkHeavy
		reasons = append(reasons, ReasonLinkHeavy)
	}

	return clampScore(score), reasons
}

func isBulkPrecedence(precedence string) bool {
	switch strings.ToLower(strings.TrimSpace(precedence)) {
	case "bulk", "junk", "list":
		return true
	default:
		return false
	}
}

func linksPerKB(body string) float64 {
	kb := float64(len(body)) / 1024
	if kb <= 0 {
		return 0
	}
	return float64(len(linkPattern.FindAllString(body, -1))) / kb
}

func clampScore(score int) int {
	if score < scoreFloor {
		return scoreFloor
	}
	if score > scoreCeiling {
		return scoreCeiling
	}
	return score
}
