//go:build tokenizers

package intent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/application/intent"
)

type zeroScorer struct{}

func (zeroScorer) TopScore(context.Context, string) (float64, error) { return 0, nil }

func TestGuardRouterClassifiesProbes(t *testing.T) {
	assetDir := os.Getenv("GUARD_ASSET_DIR")
	if assetDir == "" {
		assetDir = filepath.Join("..", "..", "..", "assets", "guard-embedder")
	}
	emb, err := embeddings.NewStaticLocalProvider(assetDir)
	if err != nil {
		t.Skipf("guard asset unavailable: %v", err)
	}
	floor, accept, margin := emb.Thresholds()

	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "guard-embedder", "probes.json"))
	if err != nil {
		t.Fatalf("read probes: %v", err)
	}
	var pf struct {
		Probes []struct {
			Text  string `json:"text"`
			Label string `json:"label"`
		} `json:"probes"`
	}
	if err := json.Unmarshal(raw, &pf); err != nil {
		t.Fatalf("parse probes: %v", err)
	}

	r, err := intent.New(context.Background(), emb, zeroScorer{},
		intent.WithThresholds(floor, accept),
		intent.WithMargin(margin),
	)
	if err != nil {
		t.Fatalf("intent.New: %v", err)
	}

	for _, p := range pf.Probes {
		d, err := r.Route(context.Background(), p.Text)
		if err != nil {
			t.Fatalf("Route(%q): %v", p.Text, err)
		}
		if (d.Intent == intent.OffDomain) != (p.Label == "off_domain") {
			t.Errorf("Route(%q) intent=%q reason=%q, want off=%v", p.Text, d.Intent, d.Reason, p.Label == "off_domain")
		}
	}
}
