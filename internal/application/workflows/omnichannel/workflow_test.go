package omnichannel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/shared/ctxkey"
)

type ctxCapturingConvos struct {
	id      int64
	created bool
	gotCtx  context.Context
}

func (s *ctxCapturingConvos) UpsertConversation(ctx context.Context, _ Conversation) (int64, bool, error) {
	s.gotCtx = ctx
	return s.id, s.created, nil
}

type ctxCapturingMessages struct {
	id     int64
	gotCtx context.Context
}

func (s *ctxCapturingMessages) InsertMessage(ctx context.Context, _ StoredMessage) (int64, error) {
	s.gotCtx = ctx
	return s.id, nil
}
func (s *ctxCapturingMessages) FindByExternalID(context.Context, string) (int64, int64, bool, error) {
	return 0, 0, false, nil
}

type foundMessages struct {
	convoID, msgID int64
	insertCalled   bool
}

func (m *foundMessages) InsertMessage(context.Context, StoredMessage) (int64, error) {
	m.insertCalled = true
	return 999, nil
}
func (m *foundMessages) FindByExternalID(context.Context, string) (int64, int64, bool, error) {
	return m.convoID, m.msgID, true, nil
}

func TestRunIsIdempotentOnDuplicateMessageID(t *testing.T) {
	convos := &ctxCapturingConvos{id: 100, created: true}
	msgs := &foundMessages{convoID: 77, msgID: 88}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: convos,
		Messages:      msgs,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := wf.Run(context.Background(), validNorm("acme@x.com"), []byte(`{}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if convos.gotCtx != nil {
		t.Fatalf("duplicate message must NOT create a new conversation")
	}
	if msgs.insertCalled {
		t.Fatalf("duplicate message must NOT insert again")
	}
	if res.ConversationID != 77 || res.MessageID != 88 {
		t.Fatalf("idempotent result must reuse existing ids, got %+v", res)
	}
}

func TestRunInjectsCompanyIntoContextForSiloWrite(t *testing.T) {
	convos := &ctxCapturingConvos{id: 100, created: true}
	msgs := &ctxCapturingMessages{id: 200}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: convos,
		Messages:      msgs,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := wf.Run(context.Background(), validNorm("acme@x.com"), []byte(`{}`)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	cid, ok := ctxkey.CompanyID(convos.gotCtx)
	if !ok || cid != 7 {
		t.Fatalf("conversation write ctx must carry resolved company_id 7 (for tenant silo routing), got cid=%d ok=%v", cid, ok)
	}
	mid, ok := ctxkey.CompanyID(msgs.gotCtx)
	if !ok || mid != 7 {
		t.Fatalf("message write ctx must carry resolved company_id 7, got cid=%d ok=%v", mid, ok)
	}
}

type stubAccounts struct {
	acc ChannelAccount
	err error
}

func (s *stubAccounts) ByChannelAndExternalID(_ context.Context, _, _ string) (ChannelAccount, error) {
	return s.acc, s.err
}

type stubConvos struct {
	id      int64
	created bool
	err     error
}

func (s *stubConvos) UpsertConversation(_ context.Context, _ Conversation) (int64, bool, error) {
	return s.id, s.created, s.err
}

type stubMessages struct {
	id  int64
	err error
}

func (s *stubMessages) InsertMessage(_ context.Context, _ StoredMessage) (int64, error) {
	return s.id, s.err
}
func (s *stubMessages) FindByExternalID(context.Context, string) (int64, int64, bool, error) {
	return 0, 0, false, nil
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

func TestRunHappyPathPersistsDraft(t *testing.T) {
	tickets := &captureTickets{}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
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
	if res.TicketEnqueued || tickets.called {
		t.Fatalf("draft must not enqueue a ticket: res=%+v tickets=%+v", res, tickets)
	}
}

func TestRunExistingConversationDoesNotEnqueue(t *testing.T) {
	tickets := &captureTickets{}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: false},
		Messages:      &stubMessages{id: 201},
		Tickets:       tickets,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := wf.Run(context.Background(), validNorm("Uabc"), []byte(`{}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.MessageID != 201 {
		t.Fatalf("expected message appended, got %+v", res)
	}
	if res.TicketEnqueued || tickets.called {
		t.Fatalf("expected no ticket enqueue for an existing open conversation: res=%+v tickets=%+v", res, tickets)
	}
}

func TestRunInboundCreatesDraftAndNeverAutoEnqueues(t *testing.T) {
	tickets := &captureTickets{}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
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
	if res.TicketEnqueued || tickets.called {
		t.Fatalf("inbound must create a DRAFT, not auto-enqueue a ticket: res=%+v tickets=%+v", res, tickets)
	}
}

func TestRunWithoutEnqueuerIsStillOK(t *testing.T) {
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
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
