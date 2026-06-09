package reviewoutbox

import (
	"context"
	"errors"
	"fmt"

	"github.com/my/app/internal/domain/ports"
)

type Options struct {
	CompanyID int64
	Limit     int
}

type Result struct {
	Scanned int
	Sent    int
	Failed  int
}

type Flusher struct {
	store     ports.ReviewOutboxStore
	deliverer ports.ReviewOutboxDeliverer
}

func NewFlusher(store ports.ReviewOutboxStore, deliverer ports.ReviewOutboxDeliverer) *Flusher {
	return &Flusher{store: store, deliverer: deliverer}
}

func (f *Flusher) Run(ctx context.Context, opts Options) (Result, error) {
	if f.store == nil {
		return Result{}, errors.New("review outbox: store is required")
	}
	if f.deliverer == nil {
		return Result{}, errors.New("review outbox: deliverer is required")
	}
	if opts.CompanyID <= 0 {
		return Result{}, errors.New("review outbox: company_id must be positive")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}

	items, err := f.store.PendingReviewOutbox(ctx, opts.CompanyID, limit)
	if err != nil {
		return Result{}, fmt.Errorf("review outbox: pending: %w", err)
	}

	res := Result{Scanned: len(items)}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		ref, err := f.deliverer.PushReviewItem(ctx, item)
		if err != nil {
			res.Failed++
			if markErr := f.store.MarkReviewOutboxFailed(ctx, item.ID, err.Error()); markErr != nil {
				return res, fmt.Errorf("review outbox: mark failed: %w", markErr)
			}
			continue
		}
		if err := f.store.MarkReviewOutboxSent(ctx, item.ID, ref); err != nil {
			return res, fmt.Errorf("review outbox: mark sent: %w", err)
		}
		res.Sent++
	}
	return res, nil
}
