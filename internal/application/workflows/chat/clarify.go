package chat

import (
	"context"
	"errors"
	"strings"

	"github.com/my/app/internal/application/skeleton"
)

func (w *Workflow) clarifyViaModel(ctx context.Context, locale, question string, companyID int64, missing []string, have map[string]string) (string, bool) {
	if len(missing) == 0 || w.resolve == nil {
		return "", false
	}

	havePairs := make([]string, 0, len(have))
	for k, v := range have {
		havePairs = append(havePairs, k+"="+v)
	}

	return w.smallModelCall(ctx, w.clarifyTmpl, map[string]string{
		"language": promptLanguage(locale),
		"question": question,
		"missing":  strings.Join(missing, ", "),
		"have":     strings.Join(havePairs, ", "),
	}, companyID)
}

func missingFields(err error) []string {
	var mp skeleton.MissingParams
	if errors.As(err, &mp) {
		return mp.Fields
	}
	return nil
}
