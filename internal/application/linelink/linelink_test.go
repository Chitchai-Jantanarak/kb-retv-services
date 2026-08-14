package linelink

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	nonces map[string]struct {
		user    string
		expires time.Time
	}
	links    map[string]int64
	upserted int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		nonces: map[string]struct {
			user    string
			expires time.Time
		}{},
		links: map[string]int64{},
	}
}

func (f *fakeStore) UpsertNonce(_ context.Context, _ int64, lineUserID, nonce string, expiresAt time.Time) error {
	f.upserted++
	f.nonces[nonce] = struct {
		user    string
		expires time.Time
	}{lineUserID, expiresAt}
	return nil
}

func (f *fakeStore) LinkByNonce(_ context.Context, _ int64, nonce string, customerID int64, now time.Time) (bool, error) {
	entry, ok := f.nonces[nonce]
	if !ok || !entry.expires.After(now) {
		return false, nil
	}
	delete(f.nonces, nonce)
	f.links[entry.user] = customerID
	return true, nil
}

func (f *fakeStore) CustomerFor(_ context.Context, _ int64, lineUserID string) (int64, bool, error) {
	id, ok := f.links[lineUserID]
	return id, ok, nil
}

func TestIssueThenCompleteLinks(t *testing.T) {
	store := newFakeStore()
	svc := New(store)
	ctx := context.Background()

	nonce, err := svc.IssueNonce(ctx, 4, "Uabc123")
	if err != nil {
		t.Fatalf("IssueNonce: %v", err)
	}
	if len(nonce) != 32 {
		t.Fatalf("nonce length = %d, want 32 hex chars", len(nonce))
	}
	if err := svc.Complete(ctx, 4, nonce, 77); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	id, err := svc.Resolve(ctx, 4, "Uabc123")
	if err != nil || id != 77 {
		t.Fatalf("Resolve = %d, %v; want 77", id, err)
	}
}

func TestCompleteRejectsUnknownAndExpired(t *testing.T) {
	store := newFakeStore()
	svc := New(store)
	ctx := context.Background()

	if err := svc.Complete(ctx, 4, "deadbeef", 77); !errors.Is(err, ErrNonceInvalid) {
		t.Fatalf("unknown nonce err = %v, want ErrNonceInvalid", err)
	}

	nonce, _ := svc.IssueNonce(ctx, 4, "Uabc123")
	svc.now = func() time.Time { return time.Now().Add(NonceTTL + time.Minute) }
	if err := svc.Complete(ctx, 4, nonce, 77); !errors.Is(err, ErrNonceInvalid) {
		t.Fatalf("expired nonce err = %v, want ErrNonceInvalid", err)
	}
}

func TestNonceSingleUse(t *testing.T) {
	store := newFakeStore()
	svc := New(store)
	ctx := context.Background()

	nonce, _ := svc.IssueNonce(ctx, 4, "Uabc123")
	if err := svc.Complete(ctx, 4, nonce, 77); err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	if err := svc.Complete(ctx, 4, nonce, 88); !errors.Is(err, ErrNonceInvalid) {
		t.Fatalf("reused nonce err = %v, want ErrNonceInvalid", err)
	}
	if id, _ := svc.Resolve(ctx, 4, "Uabc123"); id != 77 {
		t.Fatalf("link overwritten by reused nonce: %d", id)
	}
}

func TestResolveUnlinked(t *testing.T) {
	svc := New(newFakeStore())
	if _, err := svc.Resolve(context.Background(), 4, "Unope"); !errors.Is(err, ErrNotLinked) {
		t.Fatalf("err = %v, want ErrNotLinked", err)
	}
}
