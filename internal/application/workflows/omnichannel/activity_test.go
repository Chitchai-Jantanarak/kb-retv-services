package omnichannel

import (
	"context"
	"errors"
	"testing"

	"github.com/my/app/internal/domain/ports"
)

type capturingActivity struct {
	entries []ports.ActivityEntry
	err     error
}

func (a *capturingActivity) RecordActivity(_ context.Context, e ports.ActivityEntry) error {
	a.entries = append(a.entries, e)
	return a.err
}

func TestRunRecordsChannelActivity(t *testing.T) {
	act := &capturingActivity{}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &ctxCapturingMessages{id: 200},
		Activity:      act,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := wf.Run(context.Background(), validNorm("Uabc"), []byte(`{}`)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(act.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(act.entries))
	}

	e := act.entries[0]
	if e.ActorType != ports.ActorTypeChannel {
		t.Fatalf("actor_type = %q, want %q", e.ActorType, ports.ActorTypeChannel)
	}
	if e.CompanyID != 7 {
		t.Fatalf("company_id = %d, want 7", e.CompanyID)
	}
	if e.SubjectID != 200 {
		t.Fatalf("subject_id = %d, want 200", e.SubjectID)
	}
	if e.SubjectType != "message" {
		t.Fatalf("subject_type = %q, want message", e.SubjectType)
	}
	if e.ActorExternal != "Uabc" {
		t.Fatalf("actor_external = %q, want Uabc", e.ActorExternal)
	}
	if e.Action != ports.ActivityCreate {
		t.Fatalf("action = %q, want create", e.Action)
	}
	if got := e.Context["conversation_id"]; got != int64(100) {
		t.Fatalf("context conversation_id = %v, want 100", got)
	}
}

func TestRunSucceedsWhenActivityRecordingFails(t *testing.T) {
	act := &capturingActivity{err: errors.New("boom")}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &ctxCapturingMessages{id: 200},
		Activity:      act,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res, err := wf.Run(context.Background(), validNorm("Uabc"), []byte(`{}`))
	if err != nil {
		t.Fatalf("Run returned error on activity failure: %v", err)
	}
	if res.MessageID != 200 {
		t.Fatalf("message_id = %d, want 200", res.MessageID)
	}
}

func TestRunWithoutActivityRecorderDoesNotPanic(t *testing.T) {
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &ctxCapturingMessages{id: 200},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := wf.Run(context.Background(), validNorm("Uabc"), []byte(`{}`)); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
