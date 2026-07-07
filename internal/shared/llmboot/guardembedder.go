package llmboot

import (
	"fmt"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/config"
)

type GuardThresholds struct {
	Floor     float64
	Accept    float64
	Margin    float64
	HasValues bool
}

type thresholdSource interface {
	Thresholds() (floor, accept, margin float64)
}

func GuardEmbedder(cfg config.Config, fallback ports.EmbeddingProvider) (ports.EmbeddingProvider, GuardThresholds, string) {
	if cfg.Chat.GuardEmbedderProvider != "static-local" {
		return fallback, GuardThresholds{}, "guard embedder disabled; using shared embedder"
	}
	_, _, prov, err := embeddings.NewProvider(embeddings.ProviderSettings{
		Provider: "static-local",
		BaseURL:  cfg.Chat.GuardEmbedderAssetDir,
	})
	if err != nil {
		return fallback, GuardThresholds{}, fmt.Sprintf("guard embedder load failed: %v; using shared embedder", err)
	}
	th := GuardThresholds{}
	if ts, ok := prov.(thresholdSource); ok {
		f, a, m := ts.Thresholds()
		if f > 0 && m > 0 {
			th = GuardThresholds{Floor: f, Accept: a, Margin: m, HasValues: true}
		}
	}
	return prov, th, "guard embedder: static-local"
}
