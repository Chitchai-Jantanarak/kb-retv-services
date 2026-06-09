package reviewoutbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/my/app/internal/domain/ports"
)

func TestFlusherSendsPendingRowsAndMarksSent(t *testing.T) {
	store := &fakeOutboxStore{
		items: []ports.ReviewOutboxItem{
			{ID: 10, CompanyID: 7, Kind: ports.ReviewKindSymptomPropose, Payload: json.RawMessage(`{"name":"jam"}`)},
		},
	}
	deliverer := &fakeOutboxDeliverer{ref: 99}
	flusher := NewFlusher(store, deliverer)

	res, err := flusher.Run(context.Background(), Options{CompanyID: 7, Limit: 5})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Scanned != 1 || res.Sent != 1 || res.Failed != 0 {
		t.Fatalf("result = %+v, want scanned=1 sent=1 failed=0", res)
	}
	if len(deliverer.items) != 1 || deliverer.items[0].ID != 10 {
		t.Fatalf("deliverer items = %+v", deliverer.items)
	}
	if len(store.sent) != 1 || store.sent[0].id != 10 || store.sent[0].ref != 99 {
		t.Fatalf("sent marks = %+v", store.sent)
	}
}

func TestFlusherMarksFailedWhenDeliveryFails(t *testing.T) {
	store := &fakeOutboxStore{
		items: []ports.ReviewOutboxItem{
			{ID: 11, CompanyID: 7, Kind: ports.ReviewKindSymptomPropose, Payload: json.RawMessage(`{"name":"jam"}`)},
		},
	}
	flusher := NewFlusher(store, &fakeOutboxDeliverer{err: errors.New("laravel down")})

	res, err := flusher.Run(context.Background(), Options{CompanyID: 7})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Scanned != 1 || res.Sent != 0 || res.Failed != 1 {
		t.Fatalf("result = %+v, want scanned=1 failed=1", res)
	}
	if len(store.failed) != 1 || store.failed[0].id != 11 || store.failed[0].reason == "" {
		t.Fatalf("failed marks = %+v", store.failed)
	}
}

type fakeOutboxStore struct {
	items  []ports.ReviewOutboxItem
	sent   []sentMark
	failed []failedMark
}

type sentMark struct {
	id  int64
	ref int64
}

type failedMark struct {
	id     int64
	reason string
}

func (s *fakeOutboxStore) PendingReviewOutbox(_ context.Context, companyID int64, _ int) ([]ports.ReviewOutboxItem, error) {
	var out []ports.ReviewOutboxItem
	for _, item := range s.items {
		if item.CompanyID == companyID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *fakeOutboxStore) MarkReviewOutboxSent(_ context.Context, id int64, laravelRef int64) error {
	s.sent = append(s.sent, sentMark{id: id, ref: laravelRef})
	return nil
}

func (s *fakeOutboxStore) MarkReviewOutboxFailed(_ context.Context, id int64, reason string) error {
	s.failed = append(s.failed, failedMark{id: id, reason: reason})
	return nil
}

type fakeOutboxDeliverer struct {
	ref   int64
	err   error
	items []ports.ReviewOutboxItem
}

func (d *fakeOutboxDeliverer) PushReviewItem(_ context.Context, item ports.ReviewOutboxItem) (int64, error) {
	d.items = append(d.items, item)
	return d.ref, d.err
}
