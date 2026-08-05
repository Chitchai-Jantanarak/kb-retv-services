package tools

import (
	"context"
	"os"
	"testing"

	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

func TestProbeRealSelectPath(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("set PROBE=1")
	}
	cfg, err := config.LoadFrom("/app/config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	catalog, err := Load(os.DirFS("/app/config/tools"), nil)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	catalog = withNegativeTool(catalog, offDomainAnchors(t))
	guard, _, _, _ := llmboot.GuardEmbedder(cfg, nil)
	if guard == nil {
		t.Fatal("guard embedder unavailable")
	}
	ctx := context.Background()
	sel, err := NewSelector(ctx, guard, catalog, WithNameSource(probeGazetteer()))
	if err != nil {
		t.Fatalf("selector: %v", err)
	}

	granted := []string{"*"}
	correct, matched, wrongMatch := 0, 0, 0
	for _, p := range langProbes {
		got, err := sel.Select(ctx, p.text, granted)
		if err != nil {
			t.Fatalf("select %q: %v", p.text, err)
		}
		want := wantID(p)
		hit := got.Matched && got.ToolID == want
		if want == offDomainToolID {
			hit = !got.Matched || got.ToolID == offDomainToolID
		}
		if hit {
			correct++
		}
		if got.Matched {
			matched++
			if !hit {
				wrongMatch++
				t.Logf("  WRONG  %-36s want=%-16s got=%-16s score=%.3f params=%v",
					p.text, shortID(want), shortID(got.ToolID), got.Score, got.Params)
			}
		} else if !hit {
			t.Logf("  MISS   %-36s want=%-16s no tool matched", p.text, shortID(want))
		}
	}
	t.Logf("")
	t.Logf("Select() real path: correct=%d/%d  fired=%d  wrong tool calls=%d",
		correct, len(langProbes), matched, wrongMatch)
}
