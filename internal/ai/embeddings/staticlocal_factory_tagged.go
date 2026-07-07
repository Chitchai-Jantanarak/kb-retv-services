//go:build tokenizers

package embeddings

import "github.com/my/app/internal/domain/ports"

func staticLocalFromSettings(settings ProviderSettings) (ports.EmbeddingProvider, error) {
	return NewStaticLocalProvider(settings.BaseURL)
}
