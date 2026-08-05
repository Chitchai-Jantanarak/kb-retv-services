package chat

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/my/app/internal/application/skeleton"
)

const clarifyTimeout = 3 * time.Second

func (w *Workflow) clarifyViaModel(ctx context.Context, locale, question string, companyID int64, missing []string, have map[string]string) (string, bool) {
	if len(missing) == 0 || w.resolve == nil {
		return "", false
	}

	havePairs := make([]string, 0, len(have))
	for k, v := range have {
		havePairs = append(havePairs, k+"="+v)
	}

	prompt, err := w.clarifyTmpl.Render(map[string]string{
		"language": promptLanguage(locale),
		"question": question,
		"missing":  strings.Join(missing, ", "),
		"have":     strings.Join(havePairs, ", "),
	})
	if err != nil {
		return "", false
	}

	provider, err := w.resolve(ctx, companyID)
	if err != nil {
		return "", false
	}

	callCtx, cancel := context.WithTimeout(ctx, clarifyTimeout)
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

func missingFields(err error) []string {
	var mp skeleton.MissingParams
	if errors.As(err, &mp) {
		return mp.Fields
	}
	return nil
}
