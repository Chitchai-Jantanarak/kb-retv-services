package linelink

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const NonceTTL = 10 * time.Minute

var (
	ErrNonceInvalid = errors.New("linelink: nonce invalid or expired")
	ErrNotLinked    = errors.New("linelink: sender not linked")
)

type Store interface {
	UpsertNonce(ctx context.Context, companyID int64, lineUserID, nonce string, expiresAt time.Time) error
	LinkByNonce(ctx context.Context, companyID int64, nonce string, customerID int64, now time.Time) (bool, error)
	CustomerFor(ctx context.Context, companyID int64, lineUserID string) (int64, bool, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func New(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

func (s *Service) IssueNonce(ctx context.Context, companyID int64, lineUserID string) (string, error) {
	if lineUserID == "" {
		return "", fmt.Errorf("linelink: line user id is required")
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("linelink: nonce: %w", err)
	}
	nonce := hex.EncodeToString(buf)
	if err := s.store.UpsertNonce(ctx, companyID, lineUserID, nonce, s.now().Add(NonceTTL)); err != nil {
		return "", err
	}
	return nonce, nil
}

func (s *Service) Complete(ctx context.Context, companyID int64, nonce string, customerID int64) error {
	if nonce == "" || customerID <= 0 {
		return ErrNonceInvalid
	}
	ok, err := s.store.LinkByNonce(ctx, companyID, nonce, customerID, s.now())
	if err != nil {
		return err
	}
	if !ok {
		return ErrNonceInvalid
	}
	return nil
}

func (s *Service) Resolve(ctx context.Context, companyID int64, lineUserID string) (int64, error) {
	id, found, err := s.store.CustomerFor(ctx, companyID, lineUserID)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, ErrNotLinked
	}
	return id, nil
}
