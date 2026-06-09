package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultEmbedMaxRetries = 6
	embedMaxBackoff        = 90 * time.Second
)

type httpRetrier struct {
	client     *http.Client
	maxRetries int
	limiter    *rate.Limiter
}

func newHTTPRetrier(maxRetries int, requestsPerMin float64, timeout time.Duration) *httpRetrier {
	r := &httpRetrier{
		client:     &http.Client{Timeout: timeout},
		maxRetries: normalizeRetries(maxRetries),
	}
	if requestsPerMin > 0 {
		r.limiter = rate.NewLimiter(rate.Limit(requestsPerMin/60.0), 1)
	}
	return r
}

func normalizeRetries(maxRetries int) int {
	switch {
	case maxRetries < 0:
		return 0
	case maxRetries == 0:
		return defaultEmbedMaxRetries
	default:
		return maxRetries
	}
}

func (r *httpRetrier) post(ctx context.Context, url string, payload []byte, headers map[string]string) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if r.limiter != nil {
			if err := r.limiter.Wait(ctx); err != nil {
				return nil, err
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		resp, err := r.client.Do(req)
		if err != nil {
			lastErr = err
			if attempt == r.maxRetries {
				return nil, err
			}
			if werr := sleepBackoff(ctx, attempt, 0); werr != nil {
				return nil, werr
			}
			continue
		}

		if isRetryableStatus(resp.StatusCode) && attempt < r.maxRetries {
			retryAfter := retryAfterFromResponse(resp)
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("embeddings returned %s", resp.Status)
			if werr := sleepBackoff(ctx, attempt, retryAfter); werr != nil {
				return nil, werr
			}
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

func isRetryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func retryAfterFromResponse(resp *http.Response) time.Duration {
	if header := strings.TrimSpace(resp.Header.Get("Retry-After")); header != "" {
		if secs, err := strconv.Atoi(header); err == nil {
			return time.Duration(secs) * time.Second
		}
		if t, err := http.ParseTime(header); err == nil {
			if d := time.Until(t); d > 0 {
				return d
			}
		}
	}

	var body struct {
		Error struct {
			Details []struct {
				Type       string `json:"@type"`
				RetryDelay string `json:"retryDelay"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err == nil {
		for _, detail := range body.Error.Details {
			if strings.Contains(detail.Type, "RetryInfo") && detail.RetryDelay != "" {
				if delay, perr := time.ParseDuration(detail.RetryDelay); perr == nil {
					return delay
				}
			}
		}
	}
	return 0
}

func sleepBackoff(ctx context.Context, attempt int, retryAfter time.Duration) error {
	wait := backoffDuration(attempt, retryAfter)
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func backoffDuration(attempt int, retryAfter time.Duration) time.Duration {
	exp := time.Duration(math.Min(float64(embedMaxBackoff), float64(time.Second)*math.Pow(2, float64(attempt))))
	jitter := time.Duration(rand.Int63n(int64(time.Second)))
	wait := exp + jitter
	if retryAfter > 0 && retryAfter+jitter > wait {
		wait = retryAfter + jitter
	}
	if wait > embedMaxBackoff {
		wait = embedMaxBackoff
	}
	return wait
}
