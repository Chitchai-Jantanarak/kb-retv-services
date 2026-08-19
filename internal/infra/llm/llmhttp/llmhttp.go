package llmhttp

import (
	"bufio"
	"context"
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

func StreamLines(ctx context.Context, body io.ReadCloser, vendor, model string, handle func(line string) (string, bool)) <-chan ports.Completion {
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
				return
			}
		}
	}()
	return out
}
