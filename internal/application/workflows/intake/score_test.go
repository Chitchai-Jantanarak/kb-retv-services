package intake_test

import (
	"strings"
	"testing"

	"github.com/my/app/internal/application/workflows/intake"
)

func hasReason(reasons []string, want string) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}

func supportBody() string {
	return strings.Repeat("The robot stops at the dock and the wheels do not turn since this morning. ", 3)
}

func TestScoreNeverSaturates(t *testing.T) {
	spec := intake.DefaultSpec()

	best, _ := intake.Score(spec,
		map[string]string{
			intake.FieldProblemDetail: "wheels do not turn",
			intake.FieldProduct:       "Bella Bot",
			intake.FieldContact:       "081-555-0100",
			intake.FieldSeverity:      "high",
		},
		nil,
		intake.Signals{Sender: "somchai@customer.co.th", Body: supportBody(), ProductChecked: true, ProductVerified: true},
	)

	worst, _ := intake.Score(spec,
		map[string]string{},
		[]string{intake.FieldProblemDetail, intake.FieldProduct},
		intake.Signals{Sender: "no-reply@newsletter.com", Body: "hi"},
	)

	if best >= 100 || best <= 0 {
		t.Fatalf("best score = %d, want strictly inside 0..100", best)
	}
	if worst >= 100 || worst <= 0 {
		t.Fatalf("worst score = %d, want strictly inside 0..100", worst)
	}
	if best <= worst {
		t.Fatalf("best %d must outrank worst %d", best, worst)
	}
}

func TestScoreLandsInTheMiddleForPartialEvidence(t *testing.T) {
	spec := intake.DefaultSpec()

	score, reasons := intake.Score(spec,
		map[string]string{
			intake.FieldProblemDetail: "not available",
			intake.FieldProduct:       "pudu",
		},
		nil,
		intake.Signals{Sender: "chitchai@gmail.com", Body: "pudu not available"},
	)

	if score <= 20 || score >= 80 {
		t.Fatalf("score = %d, want a mid-range value for partial evidence", score)
	}
	if !hasReason(reasons, intake.ReasonProductUnchecked) {
		t.Fatalf("reasons = %v, want product_unchecked", reasons)
	}
	if !hasReason(reasons, intake.ReasonBodyThin) {
		t.Fatalf("reasons = %v, want body_thin", reasons)
	}
}

func TestScoreFlagsNewsletterShapedMail(t *testing.T) {
	spec := intake.DefaultSpec()
	body := strings.Repeat("Read more at https://example.com/a and https://example.com/b now. ", 6)

	score, reasons := intake.Score(spec,
		map[string]string{intake.FieldProblemDetail: "updates inside"},
		[]string{intake.FieldProduct},
		intake.Signals{Sender: "newsletter@mobbin.com", Body: body},
	)

	if !hasReason(reasons, intake.ReasonSenderNoReply) {
		t.Fatalf("reasons = %v, want sender_no_reply", reasons)
	}
	if !hasReason(reasons, intake.ReasonRequiredMissing) {
		t.Fatalf("reasons = %v, want required_missing", reasons)
	}

	genuine, _ := intake.Score(spec,
		map[string]string{intake.FieldProblemDetail: "wheels stuck", intake.FieldProduct: "Bella Bot"},
		nil,
		intake.Signals{Sender: "somchai@customer.co.th", Body: supportBody(), ProductChecked: true, ProductVerified: true},
	)
	if genuine <= score {
		t.Fatalf("genuine support %d must outrank newsletter %d", genuine, score)
	}
}

func TestScoreRewardsVerifiedProductOverUnchecked(t *testing.T) {
	spec := intake.DefaultSpec()
	fields := map[string]string{intake.FieldProblemDetail: "wheels stuck", intake.FieldProduct: "Bella Bot"}
	sender := intake.Signals{Sender: "somchai@customer.co.th", Body: supportBody()}

	unchecked, _ := intake.Score(spec, fields, nil, sender)

	verified := sender
	verified.ProductChecked = true
	verified.ProductVerified = true
	checked, _ := intake.Score(spec, fields, nil, verified)

	if checked <= unchecked {
		t.Fatalf("verified product %d must outrank unchecked %d", checked, unchecked)
	}
}
