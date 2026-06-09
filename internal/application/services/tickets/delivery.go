package tickets

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type DeliveryConfig struct {
	BaseURL string
	Path    string
	Secret  string
	Timeout time.Duration
	Client  *http.Client
}

type Deliverer struct {
	url    string
	secret []byte
	client *http.Client
}

func NewDeliverer(cfg DeliveryConfig) (*Deliverer, error) {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if base == "" {
		return nil, errors.New("tickets delivery: laravel base_url is required")
	}
	if cfg.Secret == "" {
		return nil, errors.New("tickets delivery: webhook secret is required")
	}
	path := cfg.Path
	if path == "" {
		path = "/api/webhooks/ai/ticket-create"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return &Deliverer{
		url:    base + path,
		secret: []byte(cfg.Secret),
		client: client,
	}, nil
}

func (d *Deliverer) Deliver(ctx context.Context, body []byte) error {
	if len(body) == 0 {
		return errors.New("tickets delivery: empty body")
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, d.secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("tickets delivery: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-AI-Timestamp", timestamp)
	req.Header.Set("X-AI-Signature", sig)

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("tickets delivery: post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("tickets delivery: status %d body=%q", resp.StatusCode, string(snippet))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
