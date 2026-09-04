package laravelhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimeout   = 10 * time.Second
	defaultReadLimit = 512
)

// Config describes one signed Laravel webhook endpoint.
type Config struct {
	// ErrPrefix opens every error this client returns, so callers keep their
	// own wording (for example "tickets delivery").
	ErrPrefix string
	// DefaultPath is used when Path is empty.
	DefaultPath string
	BaseURL     string
	Path        string
	Secret      string
	Timeout     time.Duration
	// ReadLimit caps how much of the response body is read. Defaults to 512.
	ReadLimit int64
	Client    *http.Client
}

type Client struct {
	prefix    string
	url       string
	secret    []byte
	readLimit int64
	client    *http.Client
}

func New(cfg Config) (*Client, error) {
	prefix := cfg.ErrPrefix
	if prefix == "" {
		prefix = "laravel webhook"
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		return nil, fmt.Errorf("%s: laravel base_url is required", prefix)
	}
	if cfg.Secret == "" {
		return nil, fmt.Errorf("%s: webhook secret is required", prefix)
	}
	path := cfg.Path
	if path == "" {
		path = cfg.DefaultPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	readLimit := cfg.ReadLimit
	if readLimit <= 0 {
		readLimit = defaultReadLimit
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return &Client{
		prefix:    prefix,
		url:       base + path,
		secret:    []byte(cfg.Secret),
		readLimit: readLimit,
		client:    client,
	}, nil
}

// PostSigned POSTs body with the HMAC-SHA256 headers Laravel verifies and
// returns the (limited) response body. A status of 400 or above is an error.
func (c *Client) PostSigned(ctx context.Context, body []byte) ([]byte, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("%s: empty body", c.prefix)
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", c.prefix, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-AI-Timestamp", timestamp)
	req.Header.Set("X-AI-Signature", sig)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: post: %w", c.prefix, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.readLimit))
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %w", c.prefix, err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s: status %d body=%q", c.prefix, resp.StatusCode, string(raw))
	}
	return raw, nil
}
