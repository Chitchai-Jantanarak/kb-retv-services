package omnichannel

import (
	"context"
	"testing"
)

func TestMailPolicyPreventsAutomaticTicketButKeepsMessage(t *testing.T) {
	assessor := &stubAssessor{result: Completeness{Status: "ready", Classification: "new_issue", Confidence: 95, AutoCreateDisabled: true}}
	tickets := &captureTickets{}
	workflow, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
		Completeness:  assessor,
		Tickets:       tickets,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := workflow.Run(context.Background(), emailNorm(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !assessor.called {
		t.Fatal("manual creation must still assess mail")
	}
	if tickets.called || result.TicketEnqueued {
		t.Fatal("manual creation policy enqueued a ticket")
	}
}
