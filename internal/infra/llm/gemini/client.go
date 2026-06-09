package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/my/app/internal/domain/ports"
)

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

type part struct {
	Text string `json:"text"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type generationConfig struct {
	Temperature      *float32 `json:"temperature,omitempty"`
	MaxOutputTokens  int      `json:"maxOutputTokens,omitempty"`
	StopSequences    []string `json:"stopSequences,omitempty"`
	ResponseMimeType string   `json:"responseMimeType,omitempty"`
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
	return nil, errors.New("gemini: Stream not implemented")
}

func (c *Client) call(ctx context.Context, p ports.Prompt, mime string) (ports.Completion, error) {
	req := generateRequest{
		Contents: []content{
			{Role: "user", Parts: []part{{Text: p.User}}},
		},
	}
	if p.System != "" {
		req.SystemInstruction = &content{Parts: []part{{Text: p.System}}}
	}
	if p.MaxToks > 0 || p.Temp > 0 || len(p.Stop) > 0 || mime != "" {
		cfg := &generationConfig{
			MaxOutputTokens:  p.MaxToks,
			StopSequences:    p.Stop,
			ResponseMimeType: mime,
		}
		if p.Temp > 0 {
			t := p.Temp
			cfg.Temperature = &t
		}
		req.GenerationConfig = cfg
	}

	body, err := json.Marshal(req)
	if err != nil {
		return ports.Completion{}, fmt.Errorf("gemini: marshal: %w", err)
	}

	endpoint := fmt.Sprintf(
		"%s/models/%s:generateContent?key=%s",
		c.baseURL,
		url.PathEscape(c.model),
		url.QueryEscape(c.apiKey),
	)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ports.Completion{}, fmt.Errorf("gemini: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

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
		return ports.Completion{}, fmt.Errorf("gemini: status %d: %s", resp.StatusCode, string(raw))
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
	}, nil
}

var _ ports.LLMProvider = (*Client)(nil)
