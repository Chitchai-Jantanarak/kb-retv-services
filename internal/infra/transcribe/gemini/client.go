package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/my/app/internal/domain/ports"
	apperr "github.com/my/app/internal/shared/errors"
)

const (
	DefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	DefaultModel   = "gemini-2.5-flash"
	maxAudioBytes  = 10 << 20
)

type Client struct {
	apiKey  string
	model   string
	baseURL string
	http    *http.Client
}

func New(apiKey, model, baseURL string, timeout time.Duration) *Client {
	if model == "" {
		model = DefaultModel
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Client{
		apiKey:  apiKey,
		model:   model,
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: timeout},
	}
}

type inlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type transcribePart struct {
	Text       string      `json:"text,omitempty"`
	InlineData *inlineData `json:"inlineData,omitempty"`
}

type transcribeContent struct {
	Parts []transcribePart `json:"parts"`
}

type transcribeGenerationConfig struct {
	Temperature float32 `json:"temperature"`
}

type transcribeRequest struct {
	Contents         []transcribeContent         `json:"contents"`
	GenerationConfig *transcribeGenerationConfig `json:"generationConfig,omitempty"`
}

type transcribeResponse struct {
	Candidates []struct {
		Content transcribeContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) Transcribe(ctx context.Context, in ports.AudioInput) (ports.Transcript, error) {
	if len(in.Bytes) == 0 {
		return ports.Transcript{}, apperr.New(apperr.CodeInvalidInput, "transcribe: audio bytes are required")
	}
	if len(in.Bytes) > maxAudioBytes {
		return ports.Transcript{}, apperr.New(apperr.CodePayloadTooLarge, "transcribe: audio too large")
	}

	instr := "Transcribe this audio verbatim. Output only the transcription text."
	if in.LocaleHint != "" {
		instr += " The audio is likely in language: " + in.LocaleHint
	}

	req := transcribeRequest{
		Contents: []transcribeContent{
			{Parts: []transcribePart{
				{Text: instr},
				{InlineData: &inlineData{MIMEType: in.MIMEType, Data: base64.StdEncoding.EncodeToString(in.Bytes)}},
			}},
		},
		GenerationConfig: &transcribeGenerationConfig{Temperature: 0},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return ports.Transcript{}, apperr.Wrap(apperr.CodeInternal, "transcribe: marshal request", err)
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, url.PathEscape(c.model), url.QueryEscape(c.apiKey))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ports.Transcript{}, apperr.Wrap(apperr.CodeInternal, "transcribe: build request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return ports.Transcript{}, apperr.Wrap(apperr.CodeUpstreamFailed, "transcribe: http", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ports.Transcript{}, apperr.Wrap(apperr.CodeUpstreamFailed, "transcribe: read body", err)
	}
	if resp.StatusCode >= 400 {
		return ports.Transcript{}, apperr.New(apperr.CodeUpstreamFailed, fmt.Sprintf("transcribe: status %d: %s", resp.StatusCode, string(raw)))
	}

	var parsed transcribeResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ports.Transcript{}, apperr.Wrap(apperr.CodeUpstreamFailed, "transcribe: decode response", err)
	}
	if parsed.Error != nil {
		return ports.Transcript{}, apperr.New(apperr.CodeUpstreamFailed, "transcribe: api: "+parsed.Error.Message)
	}
	if len(parsed.Candidates) == 0 {
		return ports.Transcript{}, apperr.New(apperr.CodeUpstreamFailed, "transcribe: empty candidates")
	}

	var text strings.Builder
	for _, p := range parsed.Candidates[0].Content.Parts {
		text.WriteString(p.Text)
	}
	result := strings.TrimSpace(text.String())
	if result == "" {
		return ports.Transcript{}, apperr.New(apperr.CodeUpstreamFailed, "transcribe: empty transcription")
	}

	return ports.Transcript{Text: result}, nil
}

var _ ports.Transcriber = (*Client)(nil)
