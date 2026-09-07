package skeleton

import (
	"testing"

	"github.com/my/app/internal/application/tools"
)

func TestProposalSummaryPrefersConfirmOverHeadline(t *testing.T) {
	tool := tools.Tool{
		Compose: tools.Compose{
			Headline: "created case {code} from email",
			Confirm:  "promote conversation {conversation_id}",
		},
	}
	got := proposalSummary(tool, map[string]string{"conversation_id": "4821"})
	want := "promote conversation 4821"
	if got != want {
		t.Fatalf("proposalSummary = %q, want %q", got, want)
	}
}

func TestProposalSummaryFallsBackToHeadlineWhenResolvable(t *testing.T) {
	tool := tools.Tool{
		Compose: tools.Compose{
			Headline: "case {code} updated to {status}",
		},
	}
	got := proposalSummary(tool, map[string]string{"code": "REP-1", "status": "doing"})
	want := "case REP-1 updated to doing"
	if got != want {
		t.Fatalf("proposalSummary = %q, want %q", got, want)
	}
}
