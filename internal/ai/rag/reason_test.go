package rag

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/my/app/internal/infra/llm/llmerr"
)

func TestClassifyEscalation(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantReas  Reason
		wantRetry time.Duration
	}{
		{"no error is low confidence", nil, ReasonLowConfidence, 0},
		{"provider error is unavailable with retry-after", &llmerr.ProviderError{Status: 503, RetryAfter: 2 * time.Second}, ReasonLLMUnavailable, 2 * time.Second},
		{"provider error without retry-after", &llmerr.ProviderError{Status: 429}, ReasonLLMUnavailable, 0},
		{"deadline exceeded is deadline", context.DeadlineExceeded, ReasonDeadline, 0},
		{"generic error is unavailable", errors.New("boom"), ReasonLLMUnavailable, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotReason, gotRetry := classifyEscalation(tc.err)
			if gotReason != tc.wantReas {
				t.Fatalf("reason = %q, want %q", gotReason, tc.wantReas)
			}
			if gotRetry != tc.wantRetry {
				t.Fatalf("retryAfter = %v, want %v", gotRetry, tc.wantRetry)
			}
		})
	}
}
