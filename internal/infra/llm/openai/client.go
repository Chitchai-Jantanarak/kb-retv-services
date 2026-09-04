package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/llmhttp"
)

const (
	DefaultBaseURL = "https://api.openai.com/v1"
	DefaultModel   = "gpt-4o-mini"
)

type Config struct {
	APIKey      string
	BaseURL     string
	Model       string
	HTTPClient  *http.Client
	ReferrerURL string
	Title       string
}

type Client struct {
	apiKey      string
	baseURL     string
	model       string
	httpClient  *http.Client
	referrerURL string
	title       string
}

func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("openai: api key is required")
	}
	c := &Client{
		apiKey:      cfg.APIKey,
		baseURL:     cfg.BaseURL,
		model:       cfg.Model,
		httpClient:  cfg.HTTPClient,
		referrerURL: cfg.ReferrerURL,
		title:       cfg.Title,
	}
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	if c.model == "" {
		c.model = DefaultModel
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return c, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	Temperature    *float32        `json:"temperature,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Stop           []string        `json:"stop,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
	Stream         bool            `json:"stream,omitempty"`
}

type chatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Usage   chatUsage    `json:"usage"`
	Model   string       `json:"model"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (c *Client) Generate(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	return c.call(ctx, p, nil)
}

func (c *Client) GenerateJSON(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	return c.call(ctx, p, &responseFormat{Type: "json_object"})
}

func (c *Client) Stream(ctx context.Context, p ports.Prompt) (<-chan ports.Completion, error) {
	body, err := c.buildBody(p, nil, true)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream")
	if c.referrerURL != "" {
		req.Header.Set("HTTP-Referer", c.referrerURL)
	}
	if c.title != "" {
		req.Header.Set("X-Title", c.title)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: http: %w", err)
	}
	if err := llmhttp.CheckError("openai", resp); err != nil {
		return nil, err
	}

	return llmhttp.StreamLines(ctx, resp.Body, "openai", c.model, func(trimmed string) (string, bool) {
		data, ok := strings.CutPrefix(trimmed, "data:")
		if !ok {
			return "", false
		}
		data = strings.TrimSpace(data)
		if data == "" {
			return "", false
		}
		if data == "[DONE]" {
			return "", true
		}
		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return "", false
		}
		if len(chunk.Choices) == 0 {
			return "", false
		}
		return chunk.Choices[0].Delta.Content, false
	}), nil
}

func (c *Client) call(ctx context.Context, p ports.Prompt, format *responseFormat) (ports.Completion, error) {
	body, err := c.buildBody(p, format, false)
	if err != nil {
		return ports.Completion{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ports.Completion{}, fmt.Errorf("openai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if c.referrerURL != "" {
		req.Header.Set("HTTP-Referer", c.referrerURL)
	}
	if c.title != "" {
		req.Header.Set("X-Title", c.title)
	}

	raw, err := llmhttp.DoJSON(c.httpClient, req, "openai")
	if err != nil {
		return ports.Completion{}, err
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ports.Completion{}, fmt.Errorf("openai: decode: %w", err)
	}
	if parsed.Error != nil {
		return ports.Completion{}, fmt.Errorf("openai: api: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return ports.Completion{}, errors.New("openai: empty choices")
	}

	return ports.Completion{
		Text: parsed.Choices[0].Message.Content,
		Usage: ports.TokenUsage{
			Input:  parsed.Usage.PromptTokens,
			Output: parsed.Usage.CompletionTokens,
			Total:  parsed.Usage.TotalTokens,
		},
		Vendor: "openai",
		Model:  c.model,
	}, nil
}

func (c *Client) buildBody(p ports.Prompt, format *responseFormat, stream bool) ([]byte, error) {
	msgs := make([]chatMessage, 0, 2)
	if p.System != "" {
		msgs = append(msgs, chatMessage{Role: "system", Content: p.System})
	}
	msgs = append(msgs, chatMessage{Role: "user", Content: p.User})

	req := chatRequest{
		Model:          c.model,
		Messages:       msgs,
		MaxTokens:      p.MaxToks,
		Stop:           p.Stop,
		ResponseFormat: format,
		Stream:         stream,
	}
	if p.Temp > 0 {
		t := p.Temp
		req.Temperature = &t
	}
	return json.Marshal(req)
}

var _ ports.LLMProvider = (*Client)(nil)
