package omnichannel

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

type attachmentCapturingMessages struct {
	id  int64
	got StoredMessage
}

func (s *attachmentCapturingMessages) InsertMessage(_ context.Context, m StoredMessage) (int64, error) {
	s.got = m
	return s.id, nil
}
func (s *attachmentCapturingMessages) FindByExternalID(context.Context, string) (int64, int64, bool, error) {
	return 0, 0, false, nil
}

func TestRunPassesRequestAttachmentsToStoredMessage(t *testing.T) {
	msgs := &attachmentCapturingMessages{id: 200}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      msgs,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	norm := validNorm("Uabc")
	norm.Request.Attachments = []dto.AttachmentRef{{StorageKey: "s3://bucket/key.png"}}
	if _, err := wf.Run(context.Background(), norm, []byte(`{}`)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(msgs.got.Attachments) != 1 || msgs.got.Attachments[0].StorageKey != "s3://bucket/key.png" {
		t.Fatalf("StoredMessage.Attachments = %+v, want 1 with s3://bucket/key.png", msgs.got.Attachments)
	}
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

type promoterCall struct {
	companyID, conversationID, messageID int64
	ref                                   dto.AttachmentRef
}

type promoteBytesCall struct {
	companyID, conversationID, messageID int64
	externalID, mimeType                 string
	data                                 []byte
}

type capturingPromoter struct {
	calls      []promoterCall
	bytesCalls []promoteBytesCall
	err        error
}

func (p *capturingPromoter) Promote(_ context.Context, companyID, conversationID, messageID int64, ref dto.AttachmentRef) error {
	p.calls = append(p.calls, promoterCall{companyID, conversationID, messageID, ref})
	return p.err
}

func (p *capturingPromoter) PromoteBytes(_ context.Context, companyID, conversationID, messageID int64, externalID, mimeType string, data []byte) error {
	p.bytesCalls = append(p.bytesCalls, promoteBytesCall{companyID, conversationID, messageID, externalID, mimeType, data})
	return p.err
}

func lineAttachmentNorm() Normalized {
	norm := validNorm("Uabc")
	norm.Request.Body = ""
	norm.Request.Attachments = []dto.AttachmentRef{{ID: "line-msg-1", MIMEType: "image/jpeg"}}
	return norm
}

func TestRunPromotesLineImageAttachmentsAfterInsert(t *testing.T) {
	promoter := &capturingPromoter{}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
		MediaPromoter: promoter,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := wf.Run(context.Background(), lineAttachmentNorm(), []byte(`{}`)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(promoter.calls) != 1 {
		t.Fatalf("promoter calls = %d, want 1", len(promoter.calls))
	}
	call := promoter.calls[0]
	if call.companyID != 7 || call.conversationID != 100 || call.messageID != 200 {
		t.Fatalf("promoter call ids = %+v", call)
	}
	if call.ref.ID != "line-msg-1" {
		t.Fatalf("promoter call ref = %+v", call.ref)
	}
}

func TestRunMediaPromotionFailureDoesNotFailRun(t *testing.T) {
	promoter := &capturingPromoter{err: errors.New("line api down")}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
		MediaPromoter: promoter,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := wf.Run(context.Background(), lineAttachmentNorm(), []byte(`{}`))
	if err != nil {
		t.Fatalf("Run must not fail when media promotion fails: %v", err)
	}
	if res.MessageID != 200 {
		t.Fatalf("result = %+v", res)
	}
	if len(promoter.calls) != 1 {
		t.Fatalf("promoter calls = %d, want 1", len(promoter.calls))
	}
}

func TestRunPromotesEmailAttachmentsAfterInsert(t *testing.T) {
	promoter := &capturingPromoter{}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
		MediaPromoter: promoter,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	norm := emailNorm()
	norm.Attachments = []InboundAttachment{
		{Filename: "a.jpg", MIMEType: "image/jpeg", Data: []byte("img-a")},
		{Filename: "b.png", MIMEType: "image/png", Data: []byte("img-b")},
	}
	if _, err := wf.Run(context.Background(), norm, []byte(`{}`)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(promoter.bytesCalls) != 2 {
		t.Fatalf("PromoteBytes calls = %d, want 2", len(promoter.bytesCalls))
	}
	first, second := promoter.bytesCalls[0], promoter.bytesCalls[1]
	if first.externalID == second.externalID {
		t.Fatalf("expected distinct externalIDs, got %q and %q", first.externalID, second.externalID)
	}
	if first.companyID != 7 || first.conversationID != 100 || first.messageID != 200 {
		t.Fatalf("first call ids = %+v", first)
	}
	if first.mimeType != "image/jpeg" || string(first.data) != "img-a" {
		t.Fatalf("first call = %+v", first)
	}
	if second.mimeType != "image/png" || string(second.data) != "img-b" {
		t.Fatalf("second call = %+v", second)
	}
}

func TestRunEmailMediaPromotionFailureDoesNotFailRun(t *testing.T) {
	promoter := &capturingPromoter{err: errors.New("laravel down")}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
		MediaPromoter: promoter,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	norm := emailNorm()
	norm.Attachments = []InboundAttachment{{Filename: "a.jpg", MIMEType: "image/jpeg", Data: []byte("img-a")}}
	res, err := wf.Run(context.Background(), norm, []byte(`{}`))
	if err != nil {
		t.Fatalf("Run must not fail when email media promotion fails: %v", err)
	}
	if res.MessageID != 200 {
		t.Fatalf("result = %+v", res)
	}
	if len(promoter.bytesCalls) != 1 {
		t.Fatalf("PromoteBytes calls = %d, want 1", len(promoter.bytesCalls))
	}
}

func TestRunSkipsMediaPromotionOnDuplicateMessage(t *testing.T) {
	promoter := &capturingPromoter{}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &foundMessages{convoID: 77, msgID: 88},
		MediaPromoter: promoter,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := wf.Run(context.Background(), lineAttachmentNorm(), []byte(`{}`)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(promoter.calls) != 0 {
		t.Fatalf("promoter calls = %d, want 0 for dedup short-circuit", len(promoter.calls))
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
	if !res.TicketEnqueued || !tickets.called {
		t.Fatalf("new line conversation must enqueue a ticket: res=%+v tickets=%+v", res, tickets)
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

func TestRunLineDedupeHitDoesNotEnqueue(t *testing.T) {
	tickets := &captureTickets{}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &foundMessages{convoID: 77, msgID: 88},
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
		t.Fatalf("dedupe hit must not enqueue a ticket: res=%+v tickets=%+v", res, tickets)
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

func TestRunEmailCompleteIntakeEnqueuesTicket(t *testing.T) {
	tickets := &captureTickets{}
	assessor := &stubAssessor{result: Completeness{Status: "ready"}}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
		Completeness:  assessor,
		Tickets:       tickets,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := wf.Run(context.Background(), emailNorm(), []byte(`{}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !tickets.called {
		t.Fatal("email conversation with complete intake must enqueue a ticket")
	}
	if !res.TicketEnqueued {
		t.Fatalf("TicketEnqueued = false, want true: res=%+v", res)
	}
}

func TestRunEmailIncompleteIntakeDoesNotEnqueue(t *testing.T) {
	tickets := &captureTickets{}
	assessor := &stubAssessor{result: Completeness{Status: "incomplete", Missing: []string{"product"}}}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
		Completeness:  assessor,
		Tickets:       tickets,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := wf.Run(context.Background(), emailNorm(), []byte(`{}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.TicketEnqueued || tickets.called {
		t.Fatalf("incomplete email intake must not enqueue a ticket: res=%+v tickets=%+v", res, tickets)
	}
}

func TestRunTicketEnqueueFailureDoesNotFailRun(t *testing.T) {
	tickets := &captureTickets{err: errors.New("queue down")}
	act := &capturingActivity{}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
		Tickets:       tickets,
		Activity:      act,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := wf.Run(context.Background(), validNorm("Uabc"), []byte(`{}`))
	if err != nil {
		t.Fatalf("Run must not fail when ticket enqueue fails: %v", err)
	}
	if !tickets.called {
		t.Fatal("enqueuer must still be called")
	}
	if res.TicketEnqueued {
		t.Fatalf("TicketEnqueued = true, want false on enqueue error: res=%+v", res)
	}
	found := false
	for _, e := range act.entries {
		if e.Action == ActionTicketEnqueueFailed {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an activity entry recording the enqueue failure, got %+v", act.entries)
	}
}

type keyedAccounts struct {
	byExternalID map[string]ChannelAccount
}

func (s *keyedAccounts) ByChannelAndExternalID(_ context.Context, _, externalID string) (ChannelAccount, error) {
	if acc, ok := s.byExternalID[externalID]; ok {
		return acc, nil
	}
	return ChannelAccount{}, fmt.Errorf("channel_accounts: no active account for %s: %w", externalID, ErrAccountNotFound)
}

func TestRunEmailAccountCandidateFallbackResolves(t *testing.T) {
	accounts := &keyedAccounts{byExternalID: map[string]ChannelAccount{
		"support@site.com": {ID: 21, CompanyID: 9},
	}}
	wf, err := New(Config{
		Accounts:      accounts,
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	norm := emailNorm()
	norm.AccountExternalID = "unknown@elsewhere.com"
	norm.AccountCandidates = []string{"unknown@elsewhere.com", "support@site.com"}

	res, err := wf.Run(context.Background(), norm, []byte(`{}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.CompanyID != 9 {
		t.Fatalf("CompanyID = %d, want 9 (resolved via CC candidate)", res.CompanyID)
	}
}

func TestRunEmailUnresolvedPrimaryWithNoMatchingCandidateStillFails(t *testing.T) {
	accounts := &keyedAccounts{byExternalID: map[string]ChannelAccount{
		"support@site.com": {ID: 21, CompanyID: 9},
	}}
	wf, err := New(Config{
		Accounts:      accounts,
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	norm := emailNorm()
	norm.AccountExternalID = "unknown@elsewhere.com"
	norm.AccountCandidates = []string{"unknown@elsewhere.com", "still-unknown@elsewhere.com"}

	_, err = wf.Run(context.Background(), norm, []byte(`{}`))
	if err == nil || !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("err = %v, want ErrAccountNotFound when no candidate resolves", err)
	}
}

type threadHit struct {
	convoID, msgID int64
}

type threadAwareMessages struct {
	byExternalID    map[string]threadHit
	insertCalled    bool
	insertedConvoID int64
}

func (m *threadAwareMessages) FindByExternalID(_ context.Context, externalID string) (int64, int64, bool, error) {
	if hit, ok := m.byExternalID[externalID]; ok {
		return hit.convoID, hit.msgID, true, nil
	}
	return 0, 0, false, nil
}

func (m *threadAwareMessages) InsertMessage(_ context.Context, msg StoredMessage) (int64, error) {
	m.insertCalled = true
	m.insertedConvoID = msg.ConversationID
	return 555, nil
}

func TestRunEmailReplyReusesThreadConversation(t *testing.T) {
	msgs := &threadAwareMessages{byExternalID: map[string]threadHit{
		"<m-1@x>": {convoID: 42, msgID: 1},
	}}
	convos := &ctxCapturingConvos{id: 999, created: true}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: convos,
		Messages:      msgs,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	norm := emailNorm()
	norm.Request.ExternalMessageID = "<m-2@x>"
	norm.InReplyTo = "<m-1@x>"

	res, err := wf.Run(context.Background(), norm, []byte(`{}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ConversationID != 42 {
		t.Fatalf("ConversationID = %d, want 42 (reused thread parent's conversation)", res.ConversationID)
	}
	if convos.gotCtx != nil {
		t.Fatalf("threaded reply must NOT create a new conversation")
	}
	if msgs.insertedConvoID != 42 {
		t.Fatalf("inserted message conversation_id = %d, want 42", msgs.insertedConvoID)
	}
}

func TestRunEmailReferencesFallbackReusesThreadConversation(t *testing.T) {
	msgs := &threadAwareMessages{byExternalID: map[string]threadHit{
		"<m-0@x>": {convoID: 42, msgID: 1},
	}}
	convos := &ctxCapturingConvos{id: 999, created: true}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: convos,
		Messages:      msgs,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	norm := emailNorm()
	norm.Request.ExternalMessageID = "<m-2@x>"
	norm.InReplyTo = "<m-1@x>"
	norm.References = []string{"<m-0@x>", "<m-1@x>"}

	res, err := wf.Run(context.Background(), norm, []byte(`{}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ConversationID != 42 {
		t.Fatalf("ConversationID = %d, want 42 (matched via references fallback)", res.ConversationID)
	}
	if convos.gotCtx != nil {
		t.Fatalf("threaded reply must NOT create a new conversation")
	}
}

func TestRunDetectsReferencedCaseFromSubject(t *testing.T) {
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	norm := emailNorm()
	norm.Request.Subject = "Re: REP-4106 progress?"

	res, err := wf.Run(context.Background(), norm, []byte(`{}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ReferencedCase != "REP-4106" {
		t.Fatalf("ReferencedCase = %q, want REP-4106", res.ReferencedCase)
	}
}

func TestRunReferencedCaseEmptyWhenNoRef(t *testing.T) {
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := wf.Run(context.Background(), emailNorm(), []byte(`{}`))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ReferencedCase != "" {
		t.Fatalf("ReferencedCase = %q, want empty", res.ReferencedCase)
	}
}

type stubCaseLookup struct {
	result   CaseLookupResult
	err      error
	called   bool
	coverage []int64
	code     string
}

func (s *stubCaseLookup) CaseByCode(_ context.Context, coverage []int64, code string) (CaseLookupResult, error) {
	s.called = true
	s.coverage = coverage
	s.code = code
	return s.result, s.err
}

type backfillCall struct {
	conversationID     int64
	customerID, siteID sql.NullInt64
	source             string
}

type capturingBackfill struct {
	calls []backfillCall
	err   error
}

func (b *capturingBackfill) WriteBackfill(_ context.Context, conversationID int64, customerID, siteID sql.NullInt64, source string) error {
	b.calls = append(b.calls, backfillCall{conversationID: conversationID, customerID: customerID, siteID: siteID, source: source})
	return b.err
}

func TestRunBackfillsCustomerAndSiteFromReferencedCase(t *testing.T) {
	lookup := &stubCaseLookup{result: CaseLookupResult{
		CustomerID: sql.NullInt64{Int64: 42, Valid: true},
		SiteID:     sql.NullInt64{Int64: 7, Valid: true},
	}}
	backfill := &capturingBackfill{}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
		CaseLookup:    lookup,
		Backfill:      backfill,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	norm := emailNorm()
	norm.Request.Subject = "Re: REP-4106 progress?"

	if _, err := wf.Run(context.Background(), norm, []byte(`{}`)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !lookup.called || lookup.code != "REP-4106" {
		t.Fatalf("caseLookup called = %v code = %q, want called with REP-4106", lookup.called, lookup.code)
	}
	if len(lookup.coverage) != 1 || lookup.coverage[0] != 7 {
		t.Fatalf("caseLookup coverage = %v, want [7] (company-scoped)", lookup.coverage)
	}
	if len(backfill.calls) != 1 {
		t.Fatalf("WriteBackfill calls = %d, want 1", len(backfill.calls))
	}
	call := backfill.calls[0]
	if call.conversationID != 100 {
		t.Fatalf("WriteBackfill conversationID = %d, want 100", call.conversationID)
	}
	if !call.customerID.Valid || call.customerID.Int64 != 42 {
		t.Fatalf("WriteBackfill customerID = %+v, want valid 42", call.customerID)
	}
	if !call.siteID.Valid || call.siteID.Int64 != 7 {
		t.Fatalf("WriteBackfill siteID = %+v, want valid 7", call.siteID)
	}
	if call.source != BackfillSourceReferencedCase {
		t.Fatalf("WriteBackfill source = %q, want %q", call.source, BackfillSourceReferencedCase)
	}
}

func TestRunReferencedCaseNotFoundSkipsBackfill(t *testing.T) {
	lookup := &stubCaseLookup{err: errors.New("reports: case \"REP-9999\" not found")}
	backfill := &capturingBackfill{}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
		CaseLookup:    lookup,
		Backfill:      backfill,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	norm := emailNorm()
	norm.Request.Subject = "Re: REP-9999 progress?"

	res, err := wf.Run(context.Background(), norm, []byte(`{}`))
	if err != nil {
		t.Fatalf("Run must not fail when the referenced case lookup misses: %v", err)
	}
	if res.ConversationID != 100 {
		t.Fatalf("result = %+v", res)
	}
	if !lookup.called {
		t.Fatal("caseLookup must still be called for a referenced case")
	}
	if len(backfill.calls) != 0 {
		t.Fatalf("WriteBackfill calls = %d, want 0 when the case lookup misses", len(backfill.calls))
	}
}

func TestRunNoReferencedCaseSkipsLookupAndBackfill(t *testing.T) {
	lookup := &stubCaseLookup{result: CaseLookupResult{CustomerID: sql.NullInt64{Int64: 42, Valid: true}}}
	backfill := &capturingBackfill{}
	wf, err := New(Config{
		Accounts:      &stubAccounts{acc: ChannelAccount{ID: 11, CompanyID: 7}},
		Conversations: &stubConvos{id: 100, created: true},
		Messages:      &stubMessages{id: 200},
		CaseLookup:    lookup,
		Backfill:      backfill,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := wf.Run(context.Background(), emailNorm(), []byte(`{}`)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if lookup.called {
		t.Fatal("caseLookup must not be called when there is no referenced case")
	}
	if len(backfill.calls) != 0 {
		t.Fatalf("WriteBackfill calls = %d, want 0 when there is no referenced case", len(backfill.calls))
	}
}
