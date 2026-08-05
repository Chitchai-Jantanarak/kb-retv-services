package tools

import (
	"context"
	"fmt"
	vecmath "github.com/my/app/internal/shared/vec"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

type pairSim struct {
	a, b    string
	sim     float64
	anchorA string
	anchorB string
}

func toolPairSimilarity(sel *Selector, catalog []Tool) []pairSim {
	anchorsByID := map[string][]string{}
	for _, t := range catalog {
		anchorsByID[t.ID] = t.Anchors
	}

	out := []pairSim{}
	for i := 0; i < len(sel.tools); i++ {
		for j := i + 1; j < len(sel.tools); j++ {
			ta, tb := sel.tools[i], sel.tools[j]
			best := -1.0
			bi, bj := 0, 0
			for x, va := range ta.vecs {
				for y, vb := range tb.vecs {
					if d := vecmath.Dot(va, vb); d > best {
						best, bi, bj = d, x, y
					}
				}
			}
			pa, pb := "", ""
			if list := anchorsByID[ta.id]; bi < len(list) {
				pa = list[bi]
			}
			if list := anchorsByID[tb.id]; bj < len(list) {
				pb = list[bj]
			}
			out = append(out, pairSim{a: ta.id, b: tb.id, sim: best, anchorA: pa, anchorB: pb})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].sim > out[j].sim })
	return out
}

func printMatrix(sel *Selector, label string) {
	ids := make([]string, 0, len(sel.tools))
	for _, t := range sel.tools {
		ids = append(ids, t.id)
	}

	fmt.Printf("\n---- pairwise max-anchor cosine: %s ----\n", label)
	fmt.Printf("%-22s", "")
	for _, id := range ids {
		fmt.Printf("%-6s", shortID(id))
	}
	fmt.Println()

	for i, ta := range sel.tools {
		fmt.Printf("%-22s", shortID(ids[i]))
		for _, tb := range sel.tools {
			if ta.id == tb.id {
				fmt.Printf("%-6s", "  -")
				continue
			}
			best := -1.0
			for _, va := range ta.vecs {
				for _, vb := range tb.vecs {
					if d := vecmath.Dot(va, vb); d > best {
						best = d
					}
				}
			}
			fmt.Printf("%-6.2f", best)
		}
		fmt.Println()
	}
}

func printNeighbours(pairs []pairSim, label string, top int) {
	fmt.Printf("\n---- tightest collisions: %s ----\n", label)
	for i, p := range pairs {
		if i >= top {
			break
		}
		fmt.Printf("%.4f  %-20s <-> %-20s\n", p.sim, shortID(p.a), shortID(p.b))
		fmt.Printf("          %q\n", p.anchorA)
		fmt.Printf("          %q\n", p.anchorB)
	}
}

func admissionCheck(pairs []pairSim, threshold float64) {
	fmt.Printf("\n---- admission check (pairs above %.2f) ----\n", threshold)
	worst := map[string]float64{}
	count := 0
	for _, p := range pairs {
		if p.sim < threshold {
			break
		}
		count++
		if p.sim > worst[p.a] {
			worst[p.a] = p.sim
		}
		if p.sim > worst[p.b] {
			worst[p.b] = p.sim
		}
	}
	fmt.Printf("%d colliding pairs\n", count)

	type wr struct {
		id string
		s  float64
	}
	rows := make([]wr, 0, len(worst))
	for id, s := range worst {
		rows = append(rows, wr{id, s})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].s > rows[j].s })
	fmt.Println("worst neighbour per tool:")
	for _, r := range rows {
		fmt.Printf("  %-22s %.4f\n", shortID(r.id), r.s)
	}
}

func runSimilarity(t *testing.T, label string, emb Embedder, catalog []Tool) {
	t.Helper()
	sel, err := NewSelector(context.Background(), emb, catalog)
	if err != nil {
		t.Fatalf("%s selector: %v", label, err)
	}
	fmt.Printf("\n================ %s ================\n", label)
	printMatrix(sel, label)
	pairs := toolPairSimilarity(sel, catalog)
	printNeighbours(pairs, label, 8)
	admissionCheck(pairs, 0.70)
}

func TestToolSimilarityGraph(t *testing.T) {
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
	sort.Slice(catalog, func(i, j int) bool { return catalog[i].ID < catalog[j].ID })

	_, model, shared, err := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg))
	if err != nil {
		t.Fatalf("embedder: %v", err)
	}
	runSimilarity(t, "SELECTOR "+model, shared, catalog)

	guard, _, _, _ := llmboot.GuardEmbedder(cfg, nil)
	if guard == nil {
		return
	}
	runSimilarity(t, "GUARD potion", guard, catalog)
	_ = strings.TrimSpace
}
