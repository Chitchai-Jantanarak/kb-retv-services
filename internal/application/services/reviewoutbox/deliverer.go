package reviewoutbox

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/my/app/internal/domain/ports"
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
		return nil, errors.New("review outbox: laravel base_url is required")
	}
	if cfg.Secret == "" {
		return nil, errors.New("review outbox: webhook secret is required")
	}
	path := cfg.Path
	if path == "" {
		path = "/internal/review-queue"
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

func (d *Deliverer) PushReviewItem(ctx context.Context, item ports.ReviewOutboxItem) (int64, error) {
	if item.CompanyID <= 0 {
		return 0, errors.New("review outbox: company_id must be positive")
	}
	if strings.TrimSpace(string(item.Kind)) == "" {
		return 0, errors.New("review outbox: kind is required")
	}
	if len(item.Payload) == 0 {
		return 0, errors.New("review outbox: payload is required")
	}

	body, err := json.Marshal(map[string]any{
		"company_id":   item.CompanyID,
		"kind":         item.Kind,
		"payload":      item.Payload,
		"payload_hash": strings.TrimSpace(item.PayloadHash),
	})
	if err != nil {
		return 0, fmt.Errorf("review outbox: marshal: %w", err)
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, d.secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("review outbox: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-AI-Timestamp", timestamp)
	req.Header.Set("X-AI-Signature", sig)

	resp, err := d.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("review outbox: post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return 0, fmt.Errorf("review outbox: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("review outbox: status %d body=%q", resp.StatusCode, string(raw))
	}
	return decodeLaravelRef(raw), nil
}

func decodeLaravelRef(raw []byte) int64 {
	var direct struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(raw, &direct); err == nil && direct.ID > 0 {
		return direct.ID
	}
	var enveloped struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &enveloped); err == nil && enveloped.Data.ID > 0 {
		return enveloped.Data.ID
	}
	return 0
}

var _ ports.ReviewOutboxDeliverer = (*Deliverer)(nil)
