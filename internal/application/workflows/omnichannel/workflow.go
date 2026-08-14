package omnichannel

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/tools"
	"github.com/my/app/internal/application/workflows/intake"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

const (
	BackfillSourceReferencedCase = "referenced_case"
	BackfillSourceSenderEmail    = "sender_email"
)

type CaseLookupResult struct {
	CustomerID sql.NullInt64
	SiteID     sql.NullInt64
}

type CaseLookup interface {
	CaseByCode(ctx context.Context, coverage []int64, code string) (CaseLookupResult, error)
}

type CustomerLookup interface {
	CustomerByEmailHash(ctx context.Context, coverage []int64, hash string) (sql.NullInt64, error)
}

type BackfillWriter interface {
	WriteBackfill(ctx context.Context, conversationID int64, customerID, siteID sql.NullInt64, source string) error
}

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
	Attachments       []dto.AttachmentRef
}

type MessageStore interface {
	InsertMessage(ctx context.Context, m StoredMessage) (int64, error)
	FindByExternalID(ctx context.Context, externalID string) (conversationID, messageID int64, found bool, err error)
}

type TicketEnqueuer interface {
	EnqueueTicket(ctx context.Context, companyID, conversationID, messageID int64, senderExternal string, req dto.InboundMessageRequest) error
}

type Completeness struct {
	Status         string
	Missing        []string
	Score          int
	Reasons        []string
	Classification string
	CatalogRelated *bool
}

type IntakeSignals struct {
	Sender          string
	Subject         string
	Body            string
	ListUnsubscribe bool
	AutoSubmitted   string
	Precedence      string
	HasAttachments  bool
	ReferencedCase  string
	ThreadMatched   bool
	Images          []ports.PromptImage
}

type CompletenessAssessor interface {
	Assess(ctx context.Context, companyID, conversationID int64, sig IntakeSignals) (Completeness, error)
}

type AssessEnqueuer interface {
	EnqueueAssess(ctx context.Context, companyID, conversationID, messageID int64, customer string, sig IntakeSignals, req dto.InboundMessageRequest) error
}

type MediaPromoter interface {
	Promote(ctx context.Context, companyID, conversationID, messageID int64, ref dto.AttachmentRef) error
	PromoteBytes(ctx context.Context, companyID, conversationID, messageID int64, externalID, mimeType string, data []byte) error
}

type Workflow struct {
	accounts      AccountResolver
	conversations ConversationStore
	messages      MessageStore
	tickets       TicketEnqueuer
	completeness  CompletenessAssessor
	assessQueue   AssessEnqueuer
	assessMode    string
	mediaPromoter MediaPromoter
	caseLookup     CaseLookup
	customerLookup CustomerLookup
	appKey         string
	backfill       BackfillWriter
	activity       ports.ActivityRecorder
	log            *zap.Logger
}

type Config struct {
	Accounts       AccountResolver
	Conversations  ConversationStore
	Messages       MessageStore
	Tickets        TicketEnqueuer
	Completeness   CompletenessAssessor
	AssessQueue    AssessEnqueuer
	AssessMode     string
	MediaPromoter  MediaPromoter
	CaseLookup     CaseLookup
	CustomerLookup CustomerLookup
	AppKey         string
	Backfill       BackfillWriter
	Activity       ports.ActivityRecorder
	Log            *zap.Logger
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
	log := cfg.Log
	if log == nil {
		log = zap.NewNop()
	}
	return &Workflow{
		accounts:      cfg.Accounts,
		conversations: cfg.Conversations,
		messages:      cfg.Messages,
		tickets:       cfg.Tickets,
		completeness:  cfg.Completeness,
		assessQueue:   cfg.AssessQueue,
		assessMode:    cfg.AssessMode,
		mediaPromoter: cfg.MediaPromoter,
		caseLookup:     cfg.CaseLookup,
		customerLookup: cfg.CustomerLookup,
		appKey:         cfg.AppKey,
		backfill:       cfg.Backfill,
		activity:       cfg.Activity,
		log:            log,
	}, nil
}

type Result struct {
	CompanyID      int64
	ConversationID int64
	MessageID      int64
	TicketEnqueued bool
	IntakeStatus   string
	IntakeMissing  []string
	ReferencedCase string
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

	account, err := w.resolveAccount(ctx, req.Channel, accountKey, n.AccountCandidates)
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

	var convoID int64
	var created bool
	var threadMatched bool
	if threadConvoID, threadFound := w.resolveThreadConversation(ctx, n); threadFound {
		convoID = threadConvoID
		threadMatched = true
	} else {
		convoID, created, err = w.conversations.UpsertConversation(ctx, Conversation{
			CompanyID:        account.CompanyID,
			ChannelAccountID: account.ID,
			ExternalCustomer: customer,
			Subject:          req.Subject,
			ForceNew:         req.Channel == "email",
		})
		if err != nil {
			return Result{}, fmt.Errorf("omnichannel: upsert conversation: %w", err)
		}
	}

	msgID, err := w.messages.InsertMessage(ctx, StoredMessage{
		ConversationID:    convoID,
		ExternalMessageID: req.ExternalMessageID,
		SenderExternal:    customer,
		Body:              req.Body,
		RawPayload:        raw,
		Attachments:       req.Attachments,
	})
	if err != nil {
		return Result{}, fmt.Errorf("omnichannel: insert message: %w", err)
	}

	if w.mediaPromoter != nil && req.Channel == ChannelLine && len(req.Attachments) > 0 {
		for _, ref := range req.Attachments {
			if perr := w.mediaPromoter.Promote(ctx, account.CompanyID, convoID, msgID, ref); perr != nil {
				w.log.Warn("omnichannel: media promotion failed",
					zap.Int64("company_id", account.CompanyID),
					zap.Int64("conversation_id", convoID),
					zap.Int64("message_id", msgID),
					zap.String("attachment_id", ref.ID),
					zap.Error(perr))
			}
		}
	}

	if w.mediaPromoter != nil && req.Channel == ChannelEmail && len(n.Attachments) > 0 {
		for i, att := range n.Attachments {
			externalID := fmt.Sprintf("%s#%d", req.ExternalMessageID, i)
			if perr := w.mediaPromoter.PromoteBytes(ctx, account.CompanyID, convoID, msgID, externalID, att.MIMEType, att.Data); perr != nil {
				w.log.Warn("omnichannel: media promotion failed",
					zap.Int64("company_id", account.CompanyID),
					zap.Int64("conversation_id", convoID),
					zap.Int64("message_id", msgID),
					zap.String("attachment_id", externalID),
					zap.Error(perr))
			}
		}
	}

	if w.activity != nil {
		entry := ports.ActivityEntry{
			CompanyID:     account.CompanyID,
			ActorType:     ports.ActorTypeChannel,
			ActorExternal: customer,
			SubjectType:   "message",
			SubjectID:     msgID,
			SubjectLabel:  req.Subject,
			Action:        ports.ActivityCreate,
			Context: map[string]any{
				"channel":            req.Channel,
				"channel_account_id": account.ID,
				"conversation_id":    convoID,
			},
		}
		if aerr := w.activity.RecordActivity(ctx, entry); aerr != nil {
			w.log.Warn("omnichannel: activity record failed",
				zap.Int64("company_id", account.CompanyID),
				zap.Int64("conversation_id", convoID),
				zap.Int64("message_id", msgID),
				zap.Error(aerr))
		}
	}

	res := Result{
		CompanyID:      account.CompanyID,
		ConversationID: convoID,
		MessageID:      msgID,
		ReferencedCase: detectReferencedCase(req.Subject, req.Body),
	}
	intakeClassification := ""

	customerFilled := false
	if res.ReferencedCase != "" && w.caseLookup != nil {
		customerFilled = w.backfillReferencedCase(ctx, account.CompanyID, convoID, res.ReferencedCase)
	}
	if !customerFilled && customer != "" && w.customerLookup != nil {
		w.backfillSenderCustomer(ctx, account.CompanyID, convoID, customer)
	}

	asyncEmail := w.assessMode == "async" && req.Channel == ChannelEmail && w.assessQueue != nil && created

	if req.Channel == ChannelEmail && (w.completeness != nil || asyncEmail) {
		sig := IntakeSignals{
			Sender:          customer,
			Subject:         req.Subject,
			Body:            req.Body,
			ListUnsubscribe: n.ListUnsubscribe,
			AutoSubmitted:   n.AutoSubmitted,
			Precedence:      n.Precedence,
			HasAttachments:  n.AttachmentCount > 0,
			ReferencedCase:  res.ReferencedCase,
			ThreadMatched:   threadMatched,
			Images:          attachmentImages(n.Attachments),
		}
		if asyncEmail {
			if err := w.assessQueue.EnqueueAssess(ctx, account.CompanyID, convoID, msgID, customer, sig, req); err != nil {
				w.log.Warn("omnichannel: assess enqueue failed, falling back to inline",
					zap.Int64("company_id", account.CompanyID),
					zap.Int64("conversation_id", convoID),
					zap.Error(err))
			} else {
				return res, nil
			}
		}
		if w.completeness != nil {
			if assessed, aerr := w.completeness.Assess(ctx, account.CompanyID, convoID, sig); aerr == nil {
				res.IntakeStatus = assessed.Status
				res.IntakeMissing = assessed.Missing
				intakeClassification = assessed.Classification
				w.recordIntakeAssessed(ctx, account.CompanyID, convoID, req.Subject, assessed)
			}
		}
	}

	if w.tickets != nil && created {
		promotableTicket := intakeClassification == intake.ClassificationNewIssue || intakeClassification == intake.ClassificationFollowUp
		shouldEnqueue := req.Channel == ChannelLine || (req.Channel == ChannelEmail && promotableTicket)
		if shouldEnqueue {
			if terr := w.tickets.EnqueueTicket(ctx, account.CompanyID, convoID, msgID, customer, req); terr != nil {
				w.log.Warn("omnichannel: ticket enqueue failed",
					zap.Int64("company_id", account.CompanyID),
					zap.Int64("conversation_id", convoID),
					zap.Int64("message_id", msgID),
					zap.Error(terr))
				w.recordTicketEnqueueFailed(ctx, account.CompanyID, convoID, msgID, req.Subject, terr)
			} else {
				res.TicketEnqueued = true
			}
		}
	}

	return res, nil
}

func (w *Workflow) backfillReferencedCase(ctx context.Context, companyID, convoID int64, code string) bool {
	lookup, err := w.caseLookup.CaseByCode(ctx, []int64{companyID}, code)
	if err != nil {
		w.log.Info("omnichannel: referenced case backfill lookup miss",
			zap.Int64("company_id", companyID),
			zap.Int64("conversation_id", convoID),
			zap.String("referenced_case", code),
			zap.Error(err))
		return false
	}
	if !lookup.CustomerID.Valid || w.backfill == nil {
		return false
	}
	if werr := w.backfill.WriteBackfill(ctx, convoID, lookup.CustomerID, lookup.SiteID, BackfillSourceReferencedCase); werr != nil {
		w.log.Warn("omnichannel: referenced case backfill write failed",
			zap.Int64("company_id", companyID),
			zap.Int64("conversation_id", convoID),
			zap.String("referenced_case", code),
			zap.Error(werr))
		return false
	}
	return true
}

// backfillSenderCustomer fills the customer suggestion from the sender's email
// when no referenced case supplied one. It writes a suggestion only (never the
// real customer_id) and only on an existence-guaranteed, company-scoped match.
func (w *Workflow) backfillSenderCustomer(ctx context.Context, companyID, convoID int64, sender string) {
	if w.customerLookup == nil || w.backfill == nil || w.appKey == "" {
		return
	}
	hash := EmailHash(sender, w.appKey)
	if hash == "" {
		return
	}
	customerID, err := w.customerLookup.CustomerByEmailHash(ctx, []int64{companyID}, hash)
	if err != nil {
		w.log.Info("omnichannel: sender-email backfill lookup miss",
			zap.Int64("company_id", companyID),
			zap.Int64("conversation_id", convoID),
			zap.Error(err))
		return
	}
	if !customerID.Valid {
		return
	}
	if werr := w.backfill.WriteBackfill(ctx, convoID, customerID, sql.NullInt64{}, BackfillSourceSenderEmail); werr != nil {
		w.log.Warn("omnichannel: sender-email backfill write failed",
			zap.Int64("company_id", companyID),
			zap.Int64("conversation_id", convoID),
			zap.Error(werr))
	}
}

func (w *Workflow) recordIntakeAssessed(ctx context.Context, companyID, convoID int64, subject string, assessed Completeness) {
	if w.activity == nil {
		return
	}

	entry := ports.ActivityEntry{
		CompanyID:    companyID,
		ActorType:    ports.ActorTypeAIAgent,
		SubjectType:  "conversation",
		SubjectID:    convoID,
		SubjectLabel: subject,
		Action:       ActionIntakeAssessed,
		Context: map[string]any{
			"intake_status":  assessed.Status,
			"intake_missing": assessed.Missing,
			"intake_score":   assessed.Score,
			"intake_reasons": assessed.Reasons,
		},
	}

	if err := w.activity.RecordActivity(ctx, entry); err != nil {
		w.log.Warn("omnichannel: intake activity record failed",
			zap.Int64("company_id", companyID),
			zap.Int64("conversation_id", convoID),
			zap.Error(err))
	}
}

func (w *Workflow) recordTicketEnqueueFailed(ctx context.Context, companyID, convoID, msgID int64, subject string, enqueueErr error) {
	if w.activity == nil {
		return
	}

	entry := ports.ActivityEntry{
		CompanyID:    companyID,
		ActorType:    ports.ActorTypeAIAgent,
		SubjectType:  "conversation",
		SubjectID:    convoID,
		SubjectLabel: subject,
		Action:       ActionTicketEnqueueFailed,
		Context: map[string]any{
			"message_id": msgID,
			"error":      enqueueErr.Error(),
		},
	}

	if err := w.activity.RecordActivity(ctx, entry); err != nil {
		w.log.Warn("omnichannel: ticket enqueue activity record failed",
			zap.Int64("company_id", companyID),
			zap.Int64("conversation_id", convoID),
			zap.Error(err))
	}
}

func (w *Workflow) resolveAccount(ctx context.Context, channel, primaryKey string, candidates []string) (ChannelAccount, error) {
	account, err := w.accounts.ByChannelAndExternalID(ctx, channel, primaryKey)
	if err == nil || !errors.Is(err, ErrAccountNotFound) {
		return account, err
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || candidate == primaryKey {
			continue
		}
		if acc, cerr := w.accounts.ByChannelAndExternalID(ctx, channel, candidate); cerr == nil {
			return acc, nil
		}
	}
	return account, err
}

func (w *Workflow) resolveThreadConversation(ctx context.Context, n Normalized) (int64, bool) {
	for _, ref := range threadReferences(n) {
		convoID, _, found, err := w.messages.FindByExternalID(ctx, ref)
		if err != nil {
			w.log.Warn("omnichannel: thread lookup failed",
				zap.String("reference", ref),
				zap.Error(err))
			continue
		}
		if found {
			return convoID, true
		}
	}
	return 0, false
}

func threadReferences(n Normalized) []string {
	refs := make([]string, 0, len(n.References)+1)
	if ref := strings.TrimSpace(n.InReplyTo); ref != "" {
		refs = append(refs, ref)
	}
	for _, r := range n.References {
		r = strings.TrimSpace(r)
		if r != "" {
			refs = append(refs, r)
		}
	}
	return refs
}

func attachmentImages(attachments []InboundAttachment) []ports.PromptImage {
	if len(attachments) == 0 {
		return nil
	}
	images := make([]ports.PromptImage, 0, len(attachments))
	for _, att := range attachments {
		images = append(images, ports.PromptImage{MIMEType: att.MIMEType, Data: att.Data})
	}
	return images
}

func detectReferencedCase(subject, body string) string {
	text := strings.TrimSpace(subject)
	if b := strings.TrimSpace(body); b != "" {
		text = text + "\n" + b
	}
	ref, ok := tools.ParseCaseRef(text)
	if !ok {
		return ""
	}
	return ref.Code
}
