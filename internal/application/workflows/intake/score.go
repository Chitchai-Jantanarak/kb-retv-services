package intake

import (
	"regexp"
	"strings"
)

const (
	ReasonRequiredComplete = "required_complete"
	ReasonRequiredMissing  = "required_missing"
	ReasonProductVerified  = "product_verified"
	ReasonProductUnchecked = "product_unchecked"
	ReasonProductUnknown   = "product_unknown"
	ReasonContactGiven     = "contact_given"
	ReasonContactMissing   = "contact_missing"
	ReasonSeverityGiven    = "severity_given"
	ReasonSenderNoReply    = "sender_no_reply"
	ReasonLinkHeavy        = "link_heavy"
	ReasonBodyThin         = "body_thin"
)

const (
	weightRequired = 40
	weightProduct  = 20
	weightContact  = 10
	weightSeverity = 5
	weightSender   = 15
	weightBody     = 10

	thinBodyChars   = 120
	linkHeavyPerKB  = 4.0
	scoreFloor      = 5
	scoreCeiling    = 95
)

var (
	noReplyPattern = regexp.MustCompile(`(?i)(^|[._-])(no-?reply|do-?not-?reply|newsletter|notifications?|mailer|bounce)([._-]|@)`)
	linkPattern    = regexp.MustCompile(`(?i)https?://`)
)

type Signals struct {
	Sender          string
	Body            string
	ProductVerified bool
	ProductChecked  bool
}

// Score grades an intake on deterministic evidence only. It never returns 0 or 100:
// absence of evidence is not proof, so the ends of the range stay reserved.
func Score(spec Spec, fields map[string]string, missing []string, sig Signals) (int, []string) {
	reasons := make([]string, 0, 6)
	score := 0

	if len(missing) == 0 {
		score += weightRequired
		reasons = append(reasons, ReasonRequiredComplete)
	} else {
		required := len(spec.Normalized().Required)
		if required > 0 {
			score += (weightRequired * (required - len(missing))) / required
		}
		reasons = append(reasons, ReasonRequiredMissing)
	}

	switch {
	case !sig.ProductChecked:
		reasons = append(reasons, ReasonProductUnchecked)
	case sig.ProductVerified:
		score += weightProduct
		reasons = append(reasons, ReasonProductVerified)
	default:
		reasons = append(reasons, ReasonProductUnknown)
	}

	if strings.TrimSpace(fields[FieldContact]) != "" {
		score += weightContact
		reasons = append(reasons, ReasonContactGiven)
	} else {
		reasons = append(reasons, ReasonContactMissing)
	}

	if strings.TrimSpace(fields[FieldSeverity]) != "" {
		score += weightSeverity
		reasons = append(reasons, ReasonSeverityGiven)
	}

	if noReplyPattern.MatchString(sig.Sender) {
		reasons = append(reasons, ReasonSenderNoReply)
	} else {
		score += weightSender
	}

	body := strings.TrimSpace(sig.Body)
	switch {
	case len(body) < thinBodyChars:
		reasons = append(reasons, ReasonBodyThin)
	case linksPerKB(body) >= linkHeavyPerKB:
		reasons = append(reasons, ReasonLinkHeavy)
	default:
		score += weightBody
	}

	return clampScore(score), reasons
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
