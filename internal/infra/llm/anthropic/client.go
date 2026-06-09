package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/my/app/internal/domain/ports"
)

const (
	DefaultBaseURL    = "https://api.anthropic.com/v1"
	DefaultModel      = "claude-haiku-4-5-20251001"
	DefaultAPIVersion = "2023-06-01"
	DefaultMaxTokens  = 1024

	jsonOnlySystem = "You MUST respond with a single valid JSON object. " +
		"Do not include prose, code fences, or commentary outside the JSON."
)

type Config struct {
	APIKey     string
	BaseURL    string
	Model      string
	APIVersion string
	HTTPClient *http.Client
}

type Client struct {
	apiKey     string
	baseURL    string
	model      string
	apiVersion string
	httpClient *http.Client
}

func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("anthropic: api key is required")
	}
	c := &Client{
		apiKey:     cfg.APIKey,
		baseURL:    cfg.BaseURL,
		model:      cfg.Model,
		apiVersion: cfg.APIVersion,
		httpClient: cfg.HTTPClient,
	}
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	if c.model == "" {
		c.model = DefaultModel
	}
	if c.apiVersion == "" {
		c.apiVersion = DefaultAPIVersion
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return c, nil
}

type messageContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type message struct {
	Role    string           `json:"role"`
	Content []messageContent `json:"content"`
}

type messagesRequest struct {
	Model         string    `json:"model"`
	System        string    `json:"system,omitempty"`
	Messages      []message `json:"messages"`
	MaxTokens     int       `json:"max_tokens"`
	Temperature   *float32  `json:"temperature,omitempty"`
	StopSequences []string  `json:"stop_sequences,omitempty"`
}

type messagesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type messagesResponse struct {
	Content []messageContent `json:"content"`
	Usage   messagesUsage    `json:"usage"`
	Model   string           `json:"model"`
	Error   *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) Generate(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	return c.call(ctx, p, false)
}

func (c *Client) GenerateJSON(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	return c.call(ctx, p, true)
}

func (c *Client) Stream(ctx context.Context, p ports.Prompt) (<-chan ports.Completion, error) {
	return nil, errors.New("anthropic: Stream not implemented")
}

func (c *Client) call(ctx context.Context, p ports.Prompt, forceJSON bool) (ports.Completion, error) {
	system := p.System
	if forceJSON {
		if system == "" {
			system = jsonOnlySystem
		} else {
			system = system + "\n\n" + jsonOnlySystem
		}
	}

	maxToks := p.MaxToks
	if maxToks <= 0 {
		maxToks = DefaultMaxTokens
	}

	req := messagesRequest{
		Model:         c.model,
		System:        system,
		MaxTokens:     maxToks,
		StopSequences: p.Stop,
		Messages: []message{
			{
				Role: "user",
				Content: []messageContent{
					{Type: "text", Text: p.User},
				},
			},
		},
	}
	if p.Temp > 0 {
		t := p.Temp
		req.Temperature = &t
	}

	body, err := json.Marshal(req)
	if err != nil {
		return ports.Completion{}, fmt.Errorf("anthropic: marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return ports.Completion{}, fmt.Errorf("anthropic: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", c.apiVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ports.Completion{}, fmt.Errorf("anthropic: http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ports.Completion{}, fmt.Errorf("anthropic: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return ports.Completion{}, fmt.Errorf("anthropic: status %d: %s", resp.StatusCode, string(raw))
	}

	var parsed messagesResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ports.Completion{}, fmt.Errorf("anthropic: decode: %w", err)
	}
	if parsed.Error != nil {
		return ports.Completion{}, fmt.Errorf("anthropic: api: %s", parsed.Error.Message)
	}
	if len(parsed.Content) == 0 {
		return ports.Completion{}, errors.New("anthropic: empty content")
	}

	var text strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	if text.Len() == 0 {
		return ports.Completion{}, errors.New("anthropic: no text blocks in response")
	}

	return ports.Completion{
		Text: text.String(),
		Usage: ports.TokenUsage{
			Input:  parsed.Usage.InputTokens,
			Output: parsed.Usage.OutputTokens,
			Total:  parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
		},
		Vendor: "anthropic",
	}, nil
}

var _ ports.LLMProvider = (*Client)(nil)
