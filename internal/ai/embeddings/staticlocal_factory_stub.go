//go:build !tokenizers

package embeddings

import (
	"fmt"

	"github.com/my/app/internal/domain/ports"
)

func staticLocalFromSettings(ProviderSettings) (ports.EmbeddingProvider, error) {
	return nil, fmt.Errorf("static-local embedder requires build tag 'tokenizers'")
}
