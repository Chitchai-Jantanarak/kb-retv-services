package llmboot

import (
	"context"
	"testing"

	"github.com/my/app/internal/shared/config"
)

type fakeEmb struct{ tag string }

func (f fakeEmb) Embed(context.Context, []string) ([][]float32, error) { return nil, nil }
func (f fakeEmb) Dim() int                                            { return 8 }

func TestGuardEmbedderFallsBackWhenDisabled(t *testing.T) {
	t.Parallel()
	got, th, reason := GuardEmbedder(config.Config{}, fakeEmb{tag: "shared"})
	if got.(fakeEmb).tag != "shared" || th.HasValues {
		t.Fatalf("expected fallback + no thresholds, reason=%q", reason)
	}
}

func TestGuardEmbedderFallsBackOnLoadError(t *testing.T) {
	t.Parallel()
	cfg := config.Config{}
	cfg.Chat.GuardEmbedderProvider = "static-local"
	cfg.Chat.GuardEmbedderAssetDir = "/no/such/dir"
	got, _, reason := GuardEmbedder(cfg, fakeEmb{tag: "shared"})
	if got.(fakeEmb).tag != "shared" || reason == "" {
		t.Fatalf("bad asset dir must fall back with reason, got %v reason %q", got, reason)
	}
}
