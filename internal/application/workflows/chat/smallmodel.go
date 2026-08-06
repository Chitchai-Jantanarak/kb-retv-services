package chat

import (
	"context"
	"strings"
	"time"

	"github.com/my/app/internal/ai/prompts"
)

const smallModelCallTimeout = 3 * time.Second

func (w *Workflow) smallModelCall(ctx context.Context, tmpl prompts.Template, vars map[string]string, companyID int64) (string, bool) {
	prompt, err := tmpl.Render(vars)
	if err != nil {
		return "", false
	}

	provider, err := w.resolve(ctx, companyID)
	if err != nil {
		return "", false
	}

	callCtx, cancel := context.WithTimeout(ctx, smallModelCallTimeout)
	defer cancel()
	completion, err := provider.Generate(callCtx, prompt)
	if err != nil {
		return "", false
	}

	text := strings.TrimSpace(completion.Text)
	if text == "" {
		return "", false
	}
	return text, true
}
