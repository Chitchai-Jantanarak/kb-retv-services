package llmboot

import (
	"context"
	"testing"

	"github.com/my/app/internal/shared/config"
)

type fakeEmb struct{ tag string }

func (f fakeEmb) Embed(context.Context, []string) ([][]float32, error) { return nil, nil }
func (f fakeEmb) Dim() int                                             { return 8 }

func TestGuardEmbedderFallsBackWhenDisabled(t *testing.T) {
	t.Parallel()
	got, th, reason, _ := GuardEmbedder(config.Config{}, fakeEmb{tag: "shared"})
	if got.(fakeEmb).tag != "shared" || th.HasValues {
		t.Fatalf("expected fallback + no thresholds, reason=%q", reason)
	}
}

func TestGuardEmbedderFailsClosedOnLoadError(t *testing.T) {
	t.Parallel()
	cfg := config.Config{}
	cfg.Chat.GuardEmbedderProvider = "static-local"
	cfg.Chat.GuardEmbedderAssetDir = "/no/such/dir"
	got, _, _, err := GuardEmbedder(cfg, fakeEmb{tag: "shared"})
	if err == nil {
		t.Fatal("a failed static-local load must be an error, not a silent provider substitution")
	}
	if got != nil {
		t.Fatalf("static-local must fail closed, got %v", got)
	}
}
