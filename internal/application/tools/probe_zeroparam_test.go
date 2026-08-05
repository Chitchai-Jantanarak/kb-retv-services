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

type scoredCandidate struct {
	id       string
	score    float64
	adjusted float64
	corrob   bool
}

func candidatesWithPenalty(ctx context.Context, s *Selector, text string, penalty float64, hardRank bool) []scoredCandidate {
	q := vecmath.Normalize(mustEmbedOne(ctx, s, text))
	out := make([]scoredCandidate, 0, len(s.tools))
	for _, t := range s.tools {
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
		required := requiredCount(t.params)
		corrob := required > 0 && requiredMatched(t.params, extracted) == required
		adjusted := best
		if !corrob && t.id != offDomainToolID {
			adjusted -= penalty
		}
		out = append(out, scoredCandidate{id: t.id, score: best, adjusted: adjusted, corrob: corrob})
	}
	sort.Slice(out, func(i, j int) bool {
		if hardRank && out[i].corrob != out[j].corrob {
			return out[i].corrob
		}
		return out[i].adjusted > out[j].adjusted
	})
	return out
}

func TestProbeZeroParamAsymmetry(t *testing.T) {
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

	n := len(langProbes)
	t.Logf("%-22s %8s %10s %12s", "rule", "top1", "ambig<.08", "max err margin")

	run := func(label string, penalty float64, hardRank bool) []scoredCandidate {
		correct, ambiguous := 0, 0
		maxErrMargin := 0.0
		var lastWrong []scoredCandidate
		for _, p := range langProbes {
			c := candidatesWithPenalty(ctx, sel, p.text, penalty, hardRank)
			if len(c) == 0 {
				continue
			}
			margin := 0.0
			if len(c) > 1 {
				margin = c[0].adjusted - c[1].adjusted
			}
			if c[0].id == wantID(p) {
				correct++
			} else {
				if margin > maxErrMargin {
					maxErrMargin = margin
				}
				lastWrong = c
			}
			if margin < 0.08 {
				ambiguous++
			}
		}
		t.Logf("%-22s %5d/%-3d %8d/%-3d %12.3f", label, correct, n, ambiguous, n, maxErrMargin)
		return lastWrong
	}

	run("penalty 0.00", 0.00, false)
	run("penalty 0.02", 0.02, false)
	run("penalty 0.05", 0.05, false)
	run("penalty 0.08", 0.08, false)
	run("penalty 0.12", 0.12, false)
	run("penalty 0.20", 0.20, false)
	run("hard rank", 0.00, true)

	t.Logf("")
	t.Logf("REP-code probes under hard rank:")
	for _, p := range langProbes {
		if p.want != "f2_case_status" {
			continue
		}
		c := candidatesWithPenalty(ctx, sel, p.text, 0, true)
		ids := make([]string, 0, 3)
		for i, x := range c {
			if i >= 3 {
				break
			}
			ids = append(ids, shortID(x.id))
		}
		mark := " "
		if len(c) > 0 && c[0].id != wantID(p) {
			mark = "x"
		}
		t.Logf("  %s %-36s -> %v", mark, p.text, ids)
	}
}
