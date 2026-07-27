package attachments

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/my/app/internal/domain/ports"
	apperr "github.com/my/app/internal/shared/errors"
)

const (
	defaultTimeout  = 10 * time.Second
	defaultMaxBytes = 10 << 20
)

type Client struct {
	baseURL    string
	hostHeader string
	http       *http.Client
	maxBytes   int64
}

func New(baseURL, hostHeader string, timeout time.Duration, maxBytes int64) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		hostHeader: hostHeader,
		http:       &http.Client{Timeout: timeout},
		maxBytes:   maxBytes,
	}
}

func (c *Client) Fetch(ctx context.Context, att ports.Attachment) (ports.AttachmentData, error) {
	target, err := c.resolveURL(att.URL)
	if err != nil {
		return ports.AttachmentData{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ports.AttachmentData{}, apperr.Wrap(apperr.CodeUpstreamFailed, "attachments: build request", err)
	}
	if c.hostHeader != "" {
		req.Host = c.hostHeader
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return ports.AttachmentData{}, apperr.Wrap(apperr.CodeUpstreamFailed, "attachments: http", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ports.AttachmentData{}, apperr.New(apperr.CodeUpstreamFailed, fmt.Sprintf("attachments: unexpected status %d", resp.StatusCode))
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes+1))
	if err != nil {
		return ports.AttachmentData{}, apperr.Wrap(apperr.CodeUpstreamFailed, "attachments: read body", err)
	}
	if int64(len(raw)) > c.maxBytes {
		return ports.AttachmentData{}, apperr.New(apperr.CodePayloadTooLarge, "attachment too large")
	}

	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = att.MIMEType
	}

	return ports.AttachmentData{Bytes: raw, MIMEType: mime}, nil
}

func (c *Client) resolveURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", apperr.New(apperr.CodeInvalidInput, "attachment url is required")
	}
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL, nil
	}
	if strings.HasPrefix(rawURL, "/") {
		return c.baseURL + rawURL, nil
	}
	return "", apperr.New(apperr.CodeInvalidInput, "attachment url is invalid")
}

var _ ports.AttachmentFetcher = (*Client)(nil)
