package omnichannel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/my/app/internal/application/dto"
)

type stubAccounts struct {
	acc ChannelAccount
	err error
}

func (s *stubAccounts) ByChannelAndExternalID(_ context.Context, _, _ string) (ChannelAccount, error) {
	return s.acc, s.err
}

type stubConvos struct {
	id  int64
	err error
}

func (s *stubConvos) UpsertConversation(_ context.Context, _ Conversation) (int64, error) {
	return s.id, s.err
}

type stubMessages struct {
	id  int64
	err error
}

func (s *stubMessages) InsertMessage(_ context.Context, _ StoredMessage) (int64, error) {
	return s.id, s.err
}

type captureTickets struct {
	called    bool
	companyID int64
	sender    string
	err       error
}

func (c *captureTickets) EnqueueTicket(_ context.Context, companyID, _, _ int64, sender string, _ dto.InboundMessageRequest) error {
	c.called = true
	c.companyID = companyID
	c.sender = sender
	return c.err
}

func validReq() dto.InboundMessageRequest {
	return dto.InboundMessageRequest{
		Channel:           ChannelLine,
		ExternalMessageID: "m-1",
		CustomerID:        "Uabc",
		Body:              "hello",
	}
}

func validNorm(sender string) Normalized {
	return Normalized{
		Request:           validReq(),
		ExternalSender:    sender,
		AccountExternalID: "Ubot-dest",
	}
}

func TestNewFailureCases(t *testing.T) {
	good := Config{
		Accounts:      &stubAccounts{},
		Conversations: &stubConvos{},
		Messages:      &stubMessages{},
	}
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{name: "no_accounts", mutate: func(c *Config) { c.Accounts = nil }, wantSub: "accounts"},
		{name: "no_conversations", mutate: func(c *Config) { c.Conversations = nil }, wantSub: "conversation"},
		{name: "no_messages", mutate: func(c *Config) { c.Messages = nil }, wantSub: "message"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := good
			tc.mutate(&cfg)
			_, err := New(cfg)
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %v, want sub %q", err, tc.wantSub)
			}
		})
	}
}

func TestRunRejectsEmptySender(t *testing.T) {
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 1, CompanyID: 7}},
		Conversations: &stubConvos{id: 100},
		Messages:      &stubMessages{id: 200},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = wf.Run(context.Background(), validNorm("  "), nil)
	if err == nil || !strings.Contains(err.Error(), "external sender") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunBubblesAccountResolverError(t *testing.T) {
	wf, err := New(Config{
		Accounts:      &stubAccounts{err: errors.New("db down")},
		Conversations: &stubConvos{},
		Messages:      &stubMessages{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = wf.Run(context.Background(), validNorm("Uabc"), nil)
	if err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunRejectsAccountWithNoCompany(t *testing.T) {
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 1, CompanyID: 0}},
		Conversations: &stubConvos{id: 100},
		Messages:      &stubMessages{id: 200},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = wf.Run(context.Background(), validNorm("Uabc"), nil)
	if err == nil || !strings.Contains(err.Error(), "no company") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunHappyPathPersistsAndOptionallyEnqueues(t *testing.T) {
	tickets := &captureTickets{}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100},
		Messages:      &stubMessages{id: 200},
		Tickets:       tickets,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := wf.Run(context.Background(), validNorm("Uabc"), []byte(`{}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.CompanyID != 7 || res.ConversationID != 100 || res.MessageID != 200 {
		t.Fatalf("result = %+v", res)
	}
	if !res.TicketEnqueued || !tickets.called || tickets.companyID != 7 {
		t.Fatalf("ticket enqueue not invoked: res=%+v tickets=%+v", res, tickets)
	}
}

func TestRunWithoutEnqueuerIsStillOK(t *testing.T) {
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100},
		Messages:      &stubMessages{id: 200},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := wf.Run(context.Background(), validNorm("Uabc"), nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.TicketEnqueued {
		t.Fatalf("expected ticket_enqueued=false when no enqueuer wired")
	}
}
