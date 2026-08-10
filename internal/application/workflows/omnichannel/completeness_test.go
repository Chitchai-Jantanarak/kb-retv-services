package omnichannel

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/my/app/internal/application/dto"
)

type stubAssessor struct {
	result    Completeness
	err       error
	called    bool
	companyID int64
	convoID   int64
	subject   string
	body      string
	sig       IntakeSignals
}

func (s *stubAssessor) Assess(_ context.Context, companyID, conversationID int64, sig IntakeSignals) (Completeness, error) {
	s.called = true
	s.companyID = companyID
	s.convoID = conversationID
	s.subject = sig.Subject
	s.body = sig.Body
	s.sig = sig
	return s.result, s.err
}

func emailNorm() Normalized {
	return Normalized{
		Request: dto.InboundMessageRequest{
			Channel:           ChannelEmail,
			ExternalMessageID: "<m-1@x>",
			CustomerID:        "cust@x.com",
			Subject:           "Robot stuck",
			Body:              "it will not leave the dock",
		},
		ExternalSender:    "cust@x.com",
		AccountExternalID: "desk@acme.com",
	}
}

func emailWorkflow(t *testing.T, assessor CompletenessAssessor) *Workflow {
	t.Helper()
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
		Completeness:  assessor,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return wf
}

func TestRunAssessesCompletenessOnInboundEmail(t *testing.T) {
	assessor := &stubAssessor{result: Completeness{Status: "incomplete", Missing: []string{"product"}}}
	res, err := emailWorkflow(t, assessor).Run(context.Background(), emailNorm(), []byte(`{}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !assessor.called {
		t.Fatal("email intake must be assessed for completeness")
	}
	if assessor.companyID != 7 || assessor.convoID != 100 {
		t.Fatalf("assessor got company=%d conversation=%d", assessor.companyID, assessor.convoID)
	}
	if assessor.subject != "Robot stuck" || assessor.body != "it will not leave the dock" {
		t.Fatalf("assessor got subject=%q body=%q", assessor.subject, assessor.body)
	}
	if res.IntakeStatus != "incomplete" || !reflect.DeepEqual(res.IntakeMissing, []string{"product"}) {
		t.Fatalf("result = %+v", res)
	}
	if res.TicketEnqueued {
		t.Fatal("completeness must not promote the draft")
	}
}

func TestRunSkipsCompletenessForNonEmailChannels(t *testing.T) {
	assessor := &stubAssessor{result: Completeness{Status: "ready"}}
	res, err := emailWorkflow(t, assessor).Run(context.Background(), validNorm("Uabc"), []byte(`{}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if assessor.called {
		t.Fatal("line messages must not run the email intake extractor")
	}
	if res.IntakeStatus != "" {
		t.Fatalf("IntakeStatus = %q, want empty", res.IntakeStatus)
	}
}

func TestRunPassesIntakeSignalsToAssessor(t *testing.T) {
	assessor := &stubAssessor{result: Completeness{Status: "incomplete"}}
	msgs := &threadAwareMessages{byExternalID: map[string]threadHit{
		"<m-1@x>": {convoID: 42, msgID: 1},
	}}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      msgs,
		Completeness:  assessor,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	norm := emailNorm()
	norm.Request.ExternalMessageID = "<m-2@x>"
	norm.Request.Subject = "Re: REP-4106 progress?"
	norm.InReplyTo = "<m-1@x>"
	norm.ListUnsubscribe = true

	if _, err := wf.Run(context.Background(), norm, []byte(`{}`)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !assessor.called {
		t.Fatal("assessor must be called")
	}
	if !assessor.sig.ListUnsubscribe {
		t.Fatal("ListUnsubscribe signal must reach the assessor")
	}
	if assessor.sig.ReferencedCase != "REP-4106" {
		t.Fatalf("ReferencedCase = %q, want REP-4106", assessor.sig.ReferencedCase)
	}
	if !assessor.sig.ThreadMatched {
		t.Fatal("ThreadMatched signal must reach the assessor")
	}
}

func TestRunCompletenessFailureIsNonFatalAndKeepsTheDraft(t *testing.T) {
	assessor := &stubAssessor{err: errors.New("llm down")}
	res, err := emailWorkflow(t, assessor).Run(context.Background(), emailNorm(), []byte(`{}`))
	if err != nil {
		t.Fatalf("Run must not fail when the extractor fails: %v", err)
	}
	if res.ConversationID != 100 || res.MessageID != 200 {
		t.Fatalf("draft must still land: %+v", res)
	}
	if res.IntakeStatus != "" {
		t.Fatalf("IntakeStatus = %q, want empty on failure", res.IntakeStatus)
	}
}
