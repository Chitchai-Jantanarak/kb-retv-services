package tools

import (
	"context"
	"fmt"
	vecmath "github.com/my/app/internal/shared/vec"
	"os"
	"sort"
	"testing"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

type ranked struct {
	id string
	s  float64
}

func rankAll(sel *Selector, vec []float32) []ranked {
	q := vecmath.Normalize(vec)
	out := make([]ranked, 0, len(sel.tools))
	for _, tv := range sel.tools {
		best := 0.0
		for _, v := range tv.vecs {
			if d := vecmath.Dot(q, v); d > best {
				best = d
			}
		}
		out = append(out, ranked{tv.id, best})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].s > out[j].s })
	return out
}

func dumpFailures(t *testing.T, label string, sel *Selector, vecs map[string][]float32) {
	t.Helper()
	fmt.Printf("\n================ FAILURES: %s ================\n", label)

	byTool := map[string]struct{ hit, total int }{}
	fails := 0
	for _, p := range langProbes {
		if p.want == "" {
			continue
		}
		st := byTool[p.want]
		st.total++
		r := rankAll(sel, vecs[p.text])
		if r[0].id == p.want {
			st.hit++
			byTool[p.want] = st
			continue
		}
		byTool[p.want] = st
		fails++

		wantRank, wantScore := -1, 0.0
		for i, x := range r {
			if x.id == p.want {
				wantRank, wantScore = i+1, x.s
				break
			}
		}
		fmt.Printf("\n[%s] %s\n", p.lang, p.text)
		fmt.Printf("    want %-20s rank=%d score=%.4f\n", p.want, wantRank, wantScore)
		fmt.Printf("    got  %-20s rank=1 score=%.4f   deficit=%.4f\n", r[0].id, r[0].s, r[0].s-wantScore)
		if len(r) > 1 {
			fmt.Printf("    2nd  %-20s score=%.4f\n", r[1].id, r[1].s)
		}
	}

	fmt.Printf("\n---- per-tool recall (%s) ----\n", label)
	ids := make([]string, 0, len(byTool))
	for id := range byTool {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		st := byTool[id]
		fmt.Printf("%-22s %d/%d\n", id, st.hit, st.total)
	}
	fmt.Printf("total failures: %d\n", fails)
}

func patternGated(sel *Selector, text string) map[string]bool {
	allowed := map[string]bool{}
	for _, tv := range sel.tools {
		for _, p := range tv.params {
			if p.required && p.re != nil && p.re.FindString(text) != "" {
				allowed[tv.id] = true
			}
		}
	}
	return allowed
}

func dumpGated(t *testing.T, label string, sel *Selector, vecs map[string][]float32) {
	t.Helper()
	byTool := map[string]struct{ hit, total int }{}
	gatedProbes, fails := 0, 0
	for _, p := range langProbes {
		if p.want == "" {
			continue
		}
		allowed := patternGated(sel, p.text)
		if len(allowed) > 0 {
			gatedProbes++
		}
		r := rankAll(sel, vecs[p.text])
		winner := ""
		for _, x := range r {
			if len(allowed) == 0 || allowed[x.id] {
				winner = x.id
				break
			}
		}
		st := byTool[p.want]
		st.total++
		if winner == p.want {
			st.hit++
		} else {
			fails++
		}
		byTool[p.want] = st
	}
	fmt.Printf("\n---- PATTERN-GATED recall (%s) ----\n", label)
	ids := make([]string, 0, len(byTool))
	for id := range byTool {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		st := byTool[id]
		fmt.Printf("%-22s %d/%d\n", id, st.hit, st.total)
	}
	fmt.Printf("probes narrowed by a pattern: %d   total failures: %d\n", gatedProbes, fails)
}

func TestFailureDetail(t *testing.T) {
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
	ctx := context.Background()

	_, model, shared, err := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg))
	if err != nil {
		t.Fatalf("shared embedder: %v", err)
	}
	gvecs := embedAllProbes(ctx, t, shared)
	gsel, err := NewSelector(ctx, shared, catalog)
	if err != nil {
		t.Fatalf("gemini selector: %v", err)
	}
	dumpFailures(t, "SELECTOR "+model, gsel, gvecs)
	dumpGated(t, "SELECTOR "+model, gsel, gvecs)

	guard, _, _, _ := llmboot.GuardEmbedder(cfg, nil)
	if guard == nil {
		t.Log("guard embedder unavailable")
		return
	}
	pvecs := embedAllProbes(ctx, t, guard)
	psel, err := NewSelector(ctx, guard, catalog)
	if err != nil {
		t.Fatalf("potion selector: %v", err)
	}
	dumpFailures(t, "GUARD potion", psel, pvecs)
	dumpGated(t, "GUARD potion", psel, pvecs)
}
