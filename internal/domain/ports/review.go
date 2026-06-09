package ports

import (
	"context"
	"encoding/json"
)

type ReviewKind string

const (
	ReviewKindKBPromotion    ReviewKind = "kb_promotion"
	ReviewKindSymptomPropose ReviewKind = "symptom_proposed"
	ReviewKindSubjectPropose ReviewKind = "subject_proposed"
	ReviewKindKBGap          ReviewKind = "kb_gap"
)

type ReviewItem struct {
	CompanyID int64
	Kind      ReviewKind
	Payload   any
}

type ReviewQueueWriter interface {
	Enqueue(ctx context.Context, item ReviewItem) error
}

type ReviewOutboxItem struct {
	ID          int64
	CompanyID   int64
	Kind        ReviewKind
	Payload     json.RawMessage
	PayloadHash string
	Attempts    int
}

type ReviewOutboxStore interface {
	PendingReviewOutbox(ctx context.Context, companyID int64, limit int) ([]ReviewOutboxItem, error)
	MarkReviewOutboxSent(ctx context.Context, id int64, laravelRef int64) error
	MarkReviewOutboxFailed(ctx context.Context, id int64, reason string) error
}

type ReviewOutboxDeliverer interface {
	PushReviewItem(ctx context.Context, item ReviewOutboxItem) (int64, error)
}
