package llmhttp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/infra/llm/llmerr"
)

func CheckError(vendor string, resp *http.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return &llmerr.ProviderError{
		Vendor:     vendor,
		Status:     resp.StatusCode,
		RetryAfter: llmerr.ParseRetryAfter(resp.Header.Get("Retry-After")),
		Message:    string(raw),
	}
}

func DoJSON(client *http.Client, req *http.Request, vendor string) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: http: %w", vendor, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read body: %w", vendor, err)
	}
	if resp.StatusCode >= 400 {
		return nil, &llmerr.ProviderError{
			Vendor:     vendor,
			Status:     resp.StatusCode,
			RetryAfter: llmerr.ParseRetryAfter(resp.Header.Get("Retry-After")),
			Message:    string(raw),
		}
	}
	return raw, nil
}

func StreamLines(ctx context.Context, body io.ReadCloser, vendor, model string, handle func(line string) (string, bool)) <-chan ports.Completion {
	return StreamLinesUsage(ctx, body, vendor, model, handle, nil)
}

// StreamLinesUsage is StreamLines plus a trailing usage chunk: once the body
// is drained, usage() is asked for the token counts the handler collected and,
// when non-zero, they go out as a final text-less Completion so recorders and
// the chat stream can bill a streamed reply.
func StreamLinesUsage(ctx context.Context, body io.ReadCloser, vendor, model string, handle func(line string) (string, bool), usage func() ports.TokenUsage) <-chan ports.Completion {
	out := make(chan ports.Completion)
	go func() {
		defer close(out)
		defer body.Close()
		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for scanner.Scan() {
			text, done := handle(strings.TrimSpace(scanner.Text()))
			if text != "" {
				select {
				case out <- ports.Completion{Text: text, Vendor: vendor, Model: model}:
				case <-ctx.Done():
					return
				}
			}
			if done {
				break
			}
		}
		if usage == nil {
			return
		}
		if u := usage(); u != (ports.TokenUsage{}) {
			select {
			case out <- ports.Completion{Usage: u, Vendor: vendor, Model: model}:
			case <-ctx.Done():
			}
		}
	}()
	return out
}
