package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/llmerr"
	"github.com/my/app/internal/infra/llm/llmhttp"
)

var maxPromptImages = 3
var maxPromptImageBytes = 4 << 20

const (
	DefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	DefaultModel   = "gemini-2.5-flash"
)

type Config struct {
	APIKey     string
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

type Client struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("gemini: api key is required")
	}
	c := &Client{
		apiKey:     cfg.APIKey,
		baseURL:    cfg.BaseURL,
		model:      cfg.Model,
		httpClient: cfg.HTTPClient,
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

type inlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inlineData,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type thinkingConfig struct {
	ThinkingBudget int `json:"thinkingBudget"`
}

type generationConfig struct {
	Temperature      *float32        `json:"temperature,omitempty"`
	ThinkingConfig   *thinkingConfig `json:"thinkingConfig,omitempty"`
	MaxOutputTokens  int             `json:"maxOutputTokens,omitempty"`
	StopSequences    []string        `json:"stopSequences,omitempty"`
	ResponseMimeType string          `json:"responseMimeType,omitempty"`
}

type generateRequest struct {
	Contents          []content         `json:"contents"`
	SystemInstruction *content          `json:"systemInstruction,omitempty"`
	GenerationConfig  *generationConfig `json:"generationConfig,omitempty"`
}

type generateUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type generateResponse struct {
	Candidates []struct {
		Content content `json:"content"`
	} `json:"candidates"`
	UsageMetadata generateUsage `json:"usageMetadata"`
	Error         *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (c *Client) Generate(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	return c.call(ctx, p, "")
}

func (c *Client) GenerateJSON(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	return c.call(ctx, p, "application/json")
}

func (c *Client) Stream(ctx context.Context, p ports.Prompt) (<-chan ports.Completion, error) {
	req := c.buildRequest(p, "")
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal: %w", err)
	}

	endpoint := fmt.Sprintf(
		"%s/models/%s:streamGenerateContent?alt=sse",
		c.baseURL,
		url.PathEscape(c.model),
	)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: http: %w", err)
	}
	if err := llmhttp.CheckError("gemini", resp); err != nil {
		return nil, err
	}

	return llmhttp.StreamLines(ctx, resp.Body, "gemini", c.model, func(trimmed string) (string, bool) {
		data, ok := strings.CutPrefix(trimmed, "data:")
		if !ok {
			return "", false
		}
		data = strings.TrimSpace(data)
		if data == "" {
			return "", false
		}
		var chunk generateResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return "", false
		}
		if len(chunk.Candidates) == 0 {
			return "", false
		}
		var text strings.Builder
		for _, prt := range chunk.Candidates[0].Content.Parts {
			text.WriteString(prt.Text)
		}
		return text.String(), false
	}), nil
}

func imageParts(images []ports.PromptImage) []part {
	if len(images) == 0 {
		return nil
	}
	parts := make([]part, 0, maxPromptImages)
	for _, img := range images {
		if len(parts) >= maxPromptImages {
			break
		}
		if !strings.HasPrefix(img.MIMEType, "image/") {
			continue
		}
		if len(img.Data) > maxPromptImageBytes {
			continue
		}
		parts = append(parts, part{
			InlineData: &inlineData{
				MIMEType: img.MIMEType,
				Data:     base64.StdEncoding.EncodeToString(img.Data),
			},
		})
	}
	return parts
}

func (c *Client) buildRequest(p ports.Prompt, mime string) generateRequest {
	parts := append([]part{{Text: p.User}}, imageParts(p.Images)...)
	req := generateRequest{
		Contents: []content{
			{Role: "user", Parts: parts},
		},
	}
	if p.System != "" {
		req.SystemInstruction = &content{Parts: []part{{Text: p.System}}}
	}
	if p.MaxToks > 0 || p.Temp > 0 || len(p.Stop) > 0 || mime != "" || p.ThinkBudget != nil {
		cfg := &generationConfig{
			MaxOutputTokens:  p.MaxToks,
			StopSequences:    p.Stop,
			ResponseMimeType: mime,
		}
		if p.ThinkBudget != nil {
			cfg.ThinkingConfig = &thinkingConfig{ThinkingBudget: *p.ThinkBudget}
		}
		if p.Temp > 0 {
			t := p.Temp
			cfg.Temperature = &t
		}
		req.GenerationConfig = cfg
	}
	return req
}

func (c *Client) call(ctx context.Context, p ports.Prompt, mime string) (ports.Completion, error) {
	req := c.buildRequest(p, mime)

	body, err := json.Marshal(req)
	if err != nil {
		return ports.Completion{}, fmt.Errorf("gemini: marshal: %w", err)
	}

	endpoint := fmt.Sprintf(
		"%s/models/%s:generateContent",
		c.baseURL,
		url.PathEscape(c.model),
	)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ports.Completion{}, fmt.Errorf("gemini: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ports.Completion{}, fmt.Errorf("gemini: http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ports.Completion{}, fmt.Errorf("gemini: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return ports.Completion{}, &llmerr.ProviderError{
			Vendor:     "gemini",
			Status:     resp.StatusCode,
			RetryAfter: llmerr.ParseRetryAfter(resp.Header.Get("Retry-After")),
			Message:    string(raw),
		}
	}

	var parsed generateResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ports.Completion{}, fmt.Errorf("gemini: decode: %w", err)
	}
	if parsed.Error != nil {
		return ports.Completion{}, fmt.Errorf("gemini: api: %s", parsed.Error.Message)
	}
	if len(parsed.Candidates) == 0 {
		return ports.Completion{}, errors.New("gemini: empty candidates")
	}

	var text strings.Builder
	for _, p := range parsed.Candidates[0].Content.Parts {
		text.WriteString(p.Text)
	}
	if text.Len() == 0 {
		return ports.Completion{}, errors.New("gemini: no text parts in response")
	}

	return ports.Completion{
		Text: text.String(),
		Usage: ports.TokenUsage{
			Input:  parsed.UsageMetadata.PromptTokenCount,
			Output: parsed.UsageMetadata.CandidatesTokenCount,
			Total:  parsed.UsageMetadata.TotalTokenCount,
		},
		Vendor: "gemini",
		Model:  c.model,
	}, nil
}

var _ ports.LLMProvider = (*Client)(nil)
