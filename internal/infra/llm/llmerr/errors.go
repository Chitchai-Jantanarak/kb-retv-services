package llmerr

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ProviderError struct {
	Vendor     string
	Status     int
	RetryAfter time.Duration
	Message    string
}

func (e *ProviderError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: status %d: %s", e.Vendor, e.Status, e.Message)
	}
	return fmt.Sprintf("%s: status %d", e.Vendor, e.Status)
}

func Retryable(err error) (time.Duration, bool) {
	var pe *ProviderError
	if errors.As(err, &pe) && (pe.Status == 429 || pe.Status >= 500) {
		return pe.RetryAfter, true
	}
	return 0, false
}

func ParseRetryAfter(header string) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	if secs, err := strconv.Atoi(header); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
