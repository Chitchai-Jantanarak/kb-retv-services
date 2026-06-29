package rag

import (
	"context"
	"errors"
	"time"

	"github.com/my/app/internal/infra/llm/llmerr"
)

type Reason string

const (
	ReasonNone           Reason = ""
	ReasonLowConfidence  Reason = "low_confidence"
	ReasonLLMUnavailable Reason = "llm_unavailable"
	ReasonDeadline       Reason = "deadline"
)

func classifyEscalation(err error) (Reason, time.Duration) {
	if err == nil {
		return ReasonLowConfidence, 0
	}
	var pe *llmerr.ProviderError
	if errors.As(err, &pe) {
		return ReasonLLMUnavailable, pe.RetryAfter
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ReasonDeadline, 0
	}
	return ReasonLLMUnavailable, 0
}
