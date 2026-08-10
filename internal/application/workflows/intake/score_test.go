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
	best, _ := intake.Score(intake.Signals{
		Sender:         "somchai@customer.co.th",
		Body:           supportBody(),
		ReferencedCase: "REP-4106",
		ThreadMatched:  true,
		HasAttachments: true,
		SenderKnown:    true,
	})

	worst, _ := intake.Score(intake.Signals{
		Sender:          "no-reply@newsletter.com",
		Body:            "hi",
		ListUnsubscribe: true,
		AutoSubmitted:   "auto-generated",
		Precedence:      "bulk",
	})

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

func TestScoreFlagsBulkMailBelowPlainCustomerMail(t *testing.T) {
	bulk, reasons := intake.Score(intake.Signals{
		Sender:          "no-reply@newsletter.com",
		Body:            strings.Repeat("Read more at https://example.com/a and https://example.com/b now. ", 6),
		ListUnsubscribe: true,
		AutoSubmitted:   "auto-generated",
		Precedence:      "bulk",
	})
	if !hasReason(reasons, intake.ReasonListUnsubscribe) {
		t.Fatalf("reasons = %v, want list_unsubscribe", reasons)
	}
	if !hasReason(reasons, intake.ReasonSenderNoReply) {
		t.Fatalf("reasons = %v, want sender_no_reply", reasons)
	}

	plain, _ := intake.Score(intake.Signals{
		Sender: "somchai@customer.co.th",
		Body:   "16.23 น. Robot No.1 หยุดทำงาน No.12",
	})

	if plain <= bulk {
		t.Fatalf("plain short customer mail %d must outrank bulk mail %d", plain, bulk)
	}
}

func TestScoreImageOnlyMailIsNotPenalizedLikeBulk(t *testing.T) {
	imageOnly, reasons := intake.Score(intake.Signals{
		Sender:         "somchai@customer.co.th",
		Body:           "",
		HasAttachments: true,
	})
	if !hasReason(reasons, intake.ReasonHasAttachments) {
		t.Fatalf("reasons = %v, want has_attachments", reasons)
	}

	bulk, _ := intake.Score(intake.Signals{
		Sender:          "no-reply@newsletter.com",
		Body:            "",
		ListUnsubscribe: true,
	})

	if imageOnly <= bulk {
		t.Fatalf("image-only support mail %d must outrank bulk mail %d", imageOnly, bulk)
	}
}

func TestScoreReferencedCaseOutranksNoReference(t *testing.T) {
	withRef, reasons := intake.Score(intake.Signals{
		Sender:         "somchai@customer.co.th",
		Body:           supportBody(),
		ReferencedCase: "REP-4106",
	})
	if !hasReason(reasons, intake.ReasonReferencedCase) {
		t.Fatalf("reasons = %v, want referenced_case", reasons)
	}

	withoutRef, _ := intake.Score(intake.Signals{
		Sender: "somchai@customer.co.th",
		Body:   supportBody(),
	})

	if withRef <= withoutRef {
		t.Fatalf("mail with a referenced case %d must outrank one without %d", withRef, withoutRef)
	}
}

func TestScorePrecedenceAloneNeverFlipsTheVerdict(t *testing.T) {
	plain := intake.Signals{Sender: "somchai@customer.co.th", Body: supportBody()}
	withPrecedence := plain
	withPrecedence.Precedence = "bulk"

	plainScore, _ := intake.Score(plain)
	precedenceScore, reasons := intake.Score(withPrecedence)

	if !hasReason(reasons, intake.ReasonPrecedenceBulk) {
		t.Fatalf("reasons = %v, want precedence_bulk", reasons)
	}
	if precedenceScore >= plainScore {
		t.Fatalf("precedence score %d must be lower than plain score %d", precedenceScore, plainScore)
	}

	bulk, _ := intake.Score(intake.Signals{
		Sender:          "no-reply@newsletter.com",
		Body:            supportBody(),
		ListUnsubscribe: true,
		AutoSubmitted:   "auto-generated",
	})
	if precedenceScore <= bulk {
		t.Fatalf("precedence alone must not sink a genuine mail %d to bulk-mail territory %d", precedenceScore, bulk)
	}
}

func TestScoreRewardsThreadMatchAndSenderKnown(t *testing.T) {
	unmatched, _ := intake.Score(intake.Signals{Sender: "somchai@customer.co.th", Body: supportBody()})

	matched, reasons := intake.Score(intake.Signals{
		Sender:        "somchai@customer.co.th",
		Body:          supportBody(),
		ThreadMatched: true,
		SenderKnown:   true,
	})
	if !hasReason(reasons, intake.ReasonThreadMatched) || !hasReason(reasons, intake.ReasonSenderKnown) {
		t.Fatalf("reasons = %v, want thread_matched and sender_known", reasons)
	}
	if matched <= unmatched {
		t.Fatalf("matched thread %d must outrank unmatched %d", matched, unmatched)
	}
}
