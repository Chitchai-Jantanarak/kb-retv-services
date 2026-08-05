package tools

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

func percentile(d []time.Duration, p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	i := int(float64(len(d)-1) * p)
	return d[i]
}

func TestProbeLocalPathLatency(t *testing.T) {
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

	bootStart := time.Now()
	sel, err := NewSelector(ctx, guard, catalog, WithNameSource(probeGazetteer()))
	if err != nil {
		t.Fatalf("selector: %v", err)
	}
	bootMS := time.Since(bootStart).Milliseconds()

	texts := make([]string, 0, len(langProbes))
	for _, p := range langProbes {
		texts = append(texts, p.text)
	}

	embedDur := make([]time.Duration, 0, len(texts)*5)
	for range 5 {
		for _, txt := range texts {
			start := time.Now()
			if _, err := guard.Embed(ctx, []string{txt}); err != nil {
				t.Fatalf("embed: %v", err)
			}
			embedDur = append(embedDur, time.Since(start))
		}
	}

	selectDur := make([]time.Duration, 0, len(texts)*5)
	granted := []string{"*"}
	for range 5 {
		for _, txt := range texts {
			start := time.Now()
			if _, err := sel.Select(ctx, txt, granted); err != nil {
				t.Fatalf("select: %v", err)
			}
			selectDur = append(selectDur, time.Since(start))
		}
	}

	sort.Slice(embedDur, func(i, j int) bool { return embedDur[i] < embedDur[j] })
	sort.Slice(selectDur, func(i, j int) bool { return selectDur[i] < selectDur[j] })

	t.Logf("anchor embed at boot (%d tools): %d ms one time", len(sel.tools), bootMS)
	t.Logf("%-28s %8s %8s %8s", "stage", "p50", "p95", "max")
	t.Logf("%-28s %7dus %7dus %7dus", "POTION embed one message",
		percentile(embedDur, 0.50).Microseconds(),
		percentile(embedDur, 0.95).Microseconds(),
		embedDur[len(embedDur)-1].Microseconds())
	t.Logf("%-28s %7dus %7dus %7dus", "full Select() 14 tools",
		percentile(selectDur, 0.50).Microseconds(),
		percentile(selectDur, 0.95).Microseconds(),
		selectDur[len(selectDur)-1].Microseconds())
}
