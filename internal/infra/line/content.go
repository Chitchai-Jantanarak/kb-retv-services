package line

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apperr "github.com/my/app/internal/shared/errors"
)

const (
	defaultBaseURL  = "https://api-data.line.me"
	defaultTimeout  = 15 * time.Second
	maxContentBytes = 10 << 20
)

type ContentClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func New(baseURL, token string, timeout time.Duration) *ContentClient {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &ContentClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *ContentClient) FetchContent(ctx context.Context, messageID string) ([]byte, string, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, "", apperr.New(apperr.CodeInvalidInput, "line content: message id is required")
	}
	target := fmt.Sprintf("%s/v2/bot/message/%s/content", c.baseURL, messageID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, "", apperr.Wrap(apperr.CodeUpstreamFailed, "line content: build request", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", apperr.Wrap(apperr.CodeUpstreamFailed, "line content: http", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", apperr.New(apperr.CodeUpstreamFailed, fmt.Sprintf("line content: unexpected status %d", resp.StatusCode))
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxContentBytes+1))
	if err != nil {
		return nil, "", apperr.Wrap(apperr.CodeUpstreamFailed, "line content: read body", err)
	}
	if int64(len(raw)) > maxContentBytes {
		return nil, "", apperr.New(apperr.CodePayloadTooLarge, "line content: content too large")
	}

	return raw, resp.Header.Get("Content-Type"), nil
}
