package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/llmerr"
)

const (
	FailoverOff   = "off"
	FailoverError = "error"
	FailoverQuota = "quota"
)

func NewFailover(primary, backup ports.LLMProvider, mode string) ports.LLMProvider {
	return &failover{primary: primary, backup: backup, mode: normalizeFailoverMode(mode)}
}

func normalizeFailoverMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case FailoverError:
		return FailoverError
	case FailoverQuota:
		return FailoverQuota
	default:
		return FailoverOff
	}
}

type failover struct {
	primary ports.LLMProvider
	backup  ports.LLMProvider
	mode    string
}

func (f *failover) shouldTryBackup(err error) bool {
	if err == nil || f.backup == nil {
		return false
	}
	switch f.mode {
	case FailoverError:
		return true
	case FailoverQuota:
		return isQuotaError(err)
	default:
		return false
	}
}

func (f *failover) Generate(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	out, err := f.primary.Generate(ctx, p)
	if err == nil || !f.shouldTryBackup(err) {
		return out, err
	}
	backupOut, backupErr := f.backup.Generate(ctx, p)
	if backupErr != nil {
		return ports.Completion{}, fmt.Errorf("primary: %w; backup: %v", err, backupErr)
	}
	return backupOut, nil
}

func (f *failover) GenerateJSON(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	out, err := f.primary.GenerateJSON(ctx, p)
	if err == nil || !f.shouldTryBackup(err) {
		return out, err
	}
	backupOut, backupErr := f.backup.GenerateJSON(ctx, p)
	if backupErr != nil {
		return ports.Completion{}, fmt.Errorf("primary: %w; backup: %v", err, backupErr)
	}
	return backupOut, nil
}

func (f *failover) Stream(ctx context.Context, p ports.Prompt) (<-chan ports.Completion, error) {
	ch, err := f.primary.Stream(ctx, p)
	if err == nil || !f.shouldTryBackup(err) {
		return ch, err
	}
	backupCh, backupErr := f.backup.Stream(ctx, p)
	if backupErr != nil {
		return nil, fmt.Errorf("primary: %w; backup: %v", err, backupErr)
	}
	return backupCh, nil
}

func isQuotaError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrCircuitOpen) {
		return true
	}
	var pe *llmerr.ProviderError
	if errors.As(err, &pe) && pe.Status == 429 {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "quota") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "rate-limit") || strings.Contains(msg, "too many requests")
}

var _ ports.LLMProvider = (*failover)(nil)
