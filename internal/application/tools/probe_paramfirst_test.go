package tools

import (
	"context"
	"os"
	"sort"
	"testing"

	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
	vecmath "github.com/my/app/internal/shared/vec"
)

type stubNames struct{ byLookup map[string][]string }

func (s stubNames) Names(_ context.Context, lookup string) ([]string, error) {
	return s.byLookup[lookup], nil
}

func probeGazetteer() stubNames {
	return stubNames{byLookup: map[string][]string{
		"nodes.title":            {"Bella Bot", "Flash", "Pudu Bot", "T300", "PUDU2"},
		"employees.name":         {"Somchai Prasert", "Wanida Chai"},
		"customers.company_name": {"Kantary Hotel", "Siam Michelin HQ"},
	}}
}

type eligibility struct {
	id     string
	score  float64
	params map[string]string
}

func eligibleCandidates(ctx context.Context, s *Selector, text string, paramFirst bool) []eligibility {
	q := vecmath.Normalize(mustEmbedOne(ctx, s, text))
	out := make([]eligibility, 0, len(s.tools))
	for _, t := range s.tools {
		if paramFirst {
			extracted, satisfied, err := s.extractParams(ctx, t.params, text)
			if err != nil || !satisfied {
				continue
			}
			best := 0.0
			for _, v := range t.vecs {
				if d := vecmath.Dot(q, v); d > best {
					best = d
				}
			}
			out = append(out, eligibility{id: t.id, score: best, params: extracted})
			continue
		}
		best := 0.0
		for _, v := range t.vecs {
			if d := vecmath.Dot(q, v); d > best {
				best = d
			}
		}
		out = append(out, eligibility{id: t.id, score: best})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].score > out[j].score })
	return out
}

var embedCache = map[string][]float32{}

func mustEmbedOne(ctx context.Context, s *Selector, text string) []float32 {
	if v, ok := embedCache[text]; ok {
		return v
	}
	vecs, err := s.emb.Embed(ctx, []string{text})
	if err != nil || len(vecs) != 1 {
		panic("embed failed for " + text)
	}
	embedCache[text] = vecs[0]
	return vecs[0]
}

func TestProbeParamFirstFiltering(t *testing.T) {
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

	for _, mode := range []struct {
		label      string
		paramFirst bool
	}{{"BASELINE  score all 14", false}, {"PARAM-FIRST filter then score", true}} {
		correct, ambiguous, eliminated := 0, 0, 0
		for _, p := range langProbes {
			cands := eligibleCandidates(ctx, sel, p.text, mode.paramFirst)
			if len(cands) == 0 {
				continue
			}
			eliminated += len(sel.tools) - len(cands)
			if cands[0].id == wantID(p) {
				correct++
			}
			if len(cands) > 1 && cands[0].score-cands[1].score < 0.08 {
				ambiguous++
			}
		}
		n := len(langProbes)
		t.Logf("%-32s top1=%d/%d  ambiguous(<0.08)=%d/%d  avg candidates=%.1f",
			mode.label, correct, n, ambiguous, n,
			float64(len(sel.tools)*n-eliminated)/float64(n))
	}

	t.Logf("")
	t.Logf("per-probe, param-first:")
	for _, p := range langProbes {
		cands := eligibleCandidates(ctx, sel, p.text, true)
		ids := make([]string, 0, 3)
		for i, c := range cands {
			if i >= 3 {
				break
			}
			ids = append(ids, shortID(c.id))
		}
		margin := 0.0
		if len(cands) > 1 {
			margin = cands[0].score - cands[1].score
		}
		mark := " "
		if len(cands) > 0 && cands[0].id != wantID(p) {
			mark = "x"
		}
		t.Logf("  %s %-34s want=%-16s cands=%d %v margin=%.3f",
			mark, p.text, shortID(wantID(p)), len(cands), ids, margin)
	}
}
