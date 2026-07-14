package omnichannel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/shared/ctxkey"
)

var ErrAccountNotFound = errors.New("omnichannel: channel account not found")

type ChannelAccount struct {
	ID         int64
	CompanyID  int64
	Channel    string
	ExternalID string
}

type AccountResolver interface {
	ByChannelAndExternalID(ctx context.Context, channel, externalID string) (ChannelAccount, error)
}

type Conversation struct {
	ID               int64
	CompanyID        int64
	ChannelAccountID int64
	ExternalCustomer string
	Subject          string
	ForceNew         bool
}

type ConversationStore interface {
	UpsertConversation(ctx context.Context, c Conversation) (id int64, created bool, err error)
}

type StoredMessage struct {
	ConversationID    int64
	ExternalMessageID string
	SenderExternal    string
	Body              string
	RawPayload        []byte
}

type MessageStore interface {
	InsertMessage(ctx context.Context, m StoredMessage) (int64, error)
	FindByExternalID(ctx context.Context, externalID string) (conversationID, messageID int64, found bool, err error)
}

type TicketEnqueuer interface {
	EnqueueTicket(ctx context.Context, companyID, conversationID, messageID int64, senderExternal string, req dto.InboundMessageRequest) error
}

type Completeness struct {
	Status  string
	Missing []string
}

type CompletenessAssessor interface {
	Assess(ctx context.Context, companyID, conversationID int64, subject, body string) (Completeness, error)
}

type Workflow struct {
	accounts      AccountResolver
	conversations ConversationStore
	messages      MessageStore
	tickets       TicketEnqueuer
	completeness  CompletenessAssessor
}

type Config struct {
	Accounts      AccountResolver
	Conversations ConversationStore
	Messages      MessageStore
	Tickets       TicketEnqueuer
	Completeness  CompletenessAssessor
}

func New(cfg Config) (*Workflow, error) {
	if cfg.Accounts == nil {
		return nil, errors.New("omnichannel: accounts resolver is required")
	}
	if cfg.Conversations == nil {
		return nil, errors.New("omnichannel: conversation store is required")
	}
	if cfg.Messages == nil {
		return nil, errors.New("omnichannel: message store is required")
	}
	return &Workflow{
		accounts:      cfg.Accounts,
		conversations: cfg.Conversations,
		messages:      cfg.Messages,
		tickets:       cfg.Tickets,
		completeness:  cfg.Completeness,
	}, nil
}

type Result struct {
	CompanyID      int64
	ConversationID int64
	MessageID      int64
	TicketEnqueued bool
	IntakeStatus   string
	IntakeMissing  []string
}

func (w *Workflow) Run(ctx context.Context, n Normalized, raw []byte) (Result, error) {
	req := n.Request
	req.Normalize()
	if err := req.Validate(); err != nil {
		return Result{}, fmt.Errorf("omnichannel: invalid request: %w", err)
	}
	customer := strings.TrimSpace(n.ExternalSender)
	if customer == "" {
		return Result{}, errors.New("omnichannel: external sender is required")
	}
	accountKey := strings.TrimSpace(n.AccountExternalID)
	if accountKey == "" {
		accountKey = customer
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	account, err := w.accounts.ByChannelAndExternalID(ctx, req.Channel, accountKey)
	if err != nil {
		return Result{}, fmt.Errorf("omnichannel: resolve channel account: %w", err)
	}
	if account.CompanyID <= 0 {
		return Result{}, fmt.Errorf("omnichannel: channel account %s/%s has no company: %w", req.Channel, accountKey, ErrAccountNotFound)
	}

	ctx = ctxkey.WithCompanyID(ctx, account.CompanyID)

	if extID := strings.TrimSpace(req.ExternalMessageID); extID != "" {
		convoID, msgID, found, ferr := w.messages.FindByExternalID(ctx, extID)
		if ferr != nil {
			return Result{}, fmt.Errorf("omnichannel: dedup message: %w", ferr)
		}
		if found {
			return Result{CompanyID: account.CompanyID, ConversationID: convoID, MessageID: msgID}, nil
		}
	}

	convoID, _, err := w.conversations.UpsertConversation(ctx, Conversation{
		CompanyID:        account.CompanyID,
		ChannelAccountID: account.ID,
		ExternalCustomer: customer,
		Subject:          req.Subject,
		ForceNew:         req.Channel == "email",
	})
	if err != nil {
		return Result{}, fmt.Errorf("omnichannel: upsert conversation: %w", err)
	}

	msgID, err := w.messages.InsertMessage(ctx, StoredMessage{
		ConversationID:    convoID,
		ExternalMessageID: req.ExternalMessageID,
		SenderExternal:    customer,
		Body:              req.Body,
		RawPayload:        raw,
	})
	if err != nil {
		return Result{}, fmt.Errorf("omnichannel: insert message: %w", err)
	}

	res := Result{
		CompanyID:      account.CompanyID,
		ConversationID: convoID,
		MessageID:      msgID,
	}

	if w.completeness != nil && req.Channel == ChannelEmail {
		if assessed, aerr := w.completeness.Assess(ctx, account.CompanyID, convoID, req.Subject, req.Body); aerr == nil {
			res.IntakeStatus = assessed.Status
			res.IntakeMissing = assessed.Missing
		}
	}

	return res, nil
}
