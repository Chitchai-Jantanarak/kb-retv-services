package tools

import (
	"context"
	"os"
	"sort"
	"testing"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
	vecmath "github.com/my/app/internal/shared/vec"
)

type cascadeVerdict struct {
	winner string
	margin float64
}

func topWithMargin(sel *Selector, vec []float32) cascadeVerdict {
	q := vecmath.Normalize(vec)
	type ts struct {
		id string
		s  float64
	}
	all := make([]ts, 0, len(sel.tools))
	for _, tv := range sel.tools {
		best := 0.0
		for _, v := range tv.vecs {
			if d := vecmath.Dot(q, v); d > best {
				best = d
			}
		}
		all = append(all, ts{tv.id, best})
	}
	if len(all) < 2 {
		return cascadeVerdict{}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].s > all[j].s })
	return cascadeVerdict{winner: all[0].id, margin: all[0].s - all[1].s}
}

func wantID(p langProbe) string {
	if p.want == "" {
		return offDomainToolID
	}
	return p.want
}

func TestProbeLocalFirstCascade(t *testing.T) {
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
	ctx := context.Background()

	_, model, shared, err := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg))
	if err != nil {
		t.Fatalf("shared embedder: %v", err)
	}
	guard, _, _, _ := llmboot.GuardEmbedder(cfg, nil)
	if guard == nil {
		t.Fatal("guard embedder unavailable")
	}

	gsel, err := NewSelector(ctx, shared, catalog)
	if err != nil {
		t.Fatalf("gemini selector: %v", err)
	}
	psel, err := NewSelector(ctx, guard, catalog)
	if err != nil {
		t.Fatalf("potion selector: %v", err)
	}

	texts := make([]string, len(langProbes))
	for i, p := range langProbes {
		texts[i] = p.text
	}
	gvecs, err := shared.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("gemini embed: %v", err)
	}
	pvecs, err := guard.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("potion embed: %v", err)
	}

	type row struct {
		probe  langProbe
		potion cascadeVerdict
		gemini string
		pRight bool
		gRight bool
	}
	rows := make([]row, 0, len(langProbes))
	pAll, gAll := 0, 0
	for i, p := range langProbes {
		pv := topWithMargin(psel, pvecs[i])
		gv := topWithMargin(gsel, gvecs[i])
		r := row{probe: p, potion: pv, gemini: gv.winner,
			pRight: pv.winner == wantID(p), gRight: gv.winner == wantID(p)}
		if r.pRight {
			pAll++
		}
		if r.gRight {
			gAll++
		}
		rows = append(rows, r)
	}

	n := len(rows)
	t.Logf("baseline, every probe forced:  POTION %d/%d   GEMINI %d/%d", pAll, n, gAll, n)
	t.Logf("")
	t.Logf("%8s %9s %11s %11s %11s %11s", "margin", "local%", "local ok", "local err", "deferred", "combined")

	for _, thr := range []float64{0.0, 0.02, 0.04, 0.06, 0.08, 0.10, 0.15, 0.20, 0.30} {
		local, localOK, deferred, deferredOK := 0, 0, 0, 0
		for _, r := range rows {
			if r.potion.margin >= thr {
				local++
				if r.pRight {
					localOK++
				}
			} else {
				deferred++
				if r.gRight {
					deferredOK++
				}
			}
		}
		combined := localOK + deferredOK
		t.Logf("%8.2f %8.0f%% %7d/%-3d %11d %11d %7d/%d",
			thr, 100*float64(local)/float64(n), localOK, local, local-localOK, deferred, combined, n)
	}

	t.Logf("")
	t.Logf("probes POTION gets wrong, with margin (low margin = safely deferred):")
	wrong := make([]row, 0)
	for _, r := range rows {
		if !r.pRight {
			wrong = append(wrong, r)
		}
	}
	sort.Slice(wrong, func(i, j int) bool { return wrong[i].potion.margin > wrong[j].potion.margin })
	for _, r := range wrong {
		t.Logf("  margin=%.3f  %-34s want=%-18s potion=%-18s gemini=%s",
			r.potion.margin, r.probe.text, wantID(r.probe), r.potion.winner, r.gemini)
	}
	_ = model
}
