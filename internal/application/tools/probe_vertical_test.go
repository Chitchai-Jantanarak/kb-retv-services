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

type vertical struct {
	name     string
	entities []string
	probes   []verticalProbe
}

type verticalProbe struct {
	lang string
	text string
}

var verticals = []vertical{
	{
		name:     "robotics",
		entities: []string{"Bella Bot", "Pudu Bot", "T300", "Flash", "Kitty Bot", "CC1"},
		probes: []verticalProbe{
			{"en", "cases for Bella Bot"},
			{"en", "show me issues with T300"},
			{"th", "เคสของ Bella Bot"},
			{"th", "ปัญหาของ T300 มีอะไรบ้าง"},
		},
	},
	{
		name:     "restaurant",
		entities: []string{"Pad Thai", "Green Curry", "Tom Yum Soup", "Mango Sticky Rice", "Sukhumvit Branch"},
		probes: []verticalProbe{
			{"en", "cases for Pad Thai"},
			{"en", "show me issues with Green Curry"},
			{"th", "เคสของ Pad Thai"},
			{"th", "ปัญหาของ Green Curry มีอะไรบ้าง"},
		},
	},
	{
		name:     "school",
		entities: []string{"Mathematics 101", "Physics Lab", "Building B", "Grade 7 Classroom", "Chemistry 202"},
		probes: []verticalProbe{
			{"en", "cases for Mathematics 101"},
			{"en", "show me issues with Physics Lab"},
			{"th", "เคสของ Mathematics 101"},
			{"th", "ปัญหาของ Physics Lab มีอะไรบ้าง"},
		},
	},
	{
		name:     "delivery",
		entities: []string{"Route 12", "Van A-338", "Bangkok Hub", "Cold Chain Truck", "Rider App"},
		probes: []verticalProbe{
			{"en", "cases for Route 12"},
			{"en", "show me issues with Van A-338"},
			{"th", "เคสของ Route 12"},
			{"th", "ปัญหาของ Van A-338 มีอะไรบ้าง"},
		},
	},
}

var neutralF5Anchors = []string{
	"cases for this one",
	"issues with this one",
	"problems reported about it",
	"เคสของสิ่งนี้",
	"ปัญหาของสิ่งนี้",
}

type verticalNames struct {
	entities []string
}

func (v verticalNames) Names(ctx context.Context, lookup string) ([]string, error) {
	if lookup == "nodes.title" {
		return v.entities, nil
	}
	return nil, nil
}

func neutralCatalog(catalog []Tool) []Tool {
	out := make([]Tool, 0, len(catalog))
	for _, t := range catalog {
		if t.ID == "f5_product_cases" {
			t.Anchors = neutralF5Anchors
		}
		out = append(out, t)
	}
	return out
}

func f5Reach(sel *Selector, emb Embedder, ctx context.Context, text string) (float64, float64, string) {
	vecs, _ := emb.Embed(ctx, []string{text})
	q := vecmath.Normalize(vecs[0])
	var f5Score, f5Accept, topScore float64
	topID := ""
	for _, tv := range sel.tools {
		best := 0.0
		for _, v := range tv.vecs {
			if d := vecmath.Dot(q, v); d > best {
				best = d
			}
		}
		if best > topScore {
			topScore, topID = best, tv.id
		}
		if tv.id == "f5_product_cases" {
			f5Score, f5Accept = best, tv.accept
		}
	}
	return f5Score, f5Accept, topID
}

func runVerticals(t *testing.T, label string, emb Embedder, catalog []Tool) {
	t.Helper()
	ctx := context.Background()
	grants := []string{"*"}

	fmt.Printf("\n================ %s ================\n", label)
	fmt.Printf("%-11s %-38s %-18s %-7s %-7s %s\n",
		"vertical", "probe", "Select() ->", "f5", "accept", "cosine top1")
	fmt.Println(strings.Repeat("-", 108))

	totals := map[string]struct{ hit, total int }{}
	for _, v := range verticals {
		sel, err := NewSelector(ctx, emb, catalog, WithNameSource(verticalNames{entities: v.entities}))
		if err != nil {
			t.Fatalf("%s selector: %v", v.name, err)
		}
		for _, p := range v.probes {
			got, err := sel.Select(ctx, p.text, grants)
			if err != nil {
				t.Fatalf("select %q: %v", p.text, err)
			}
			f5, accept, top := f5Reach(sel, emb, ctx, p.text)

			picked := got.ToolID
			if !got.Matched {
				picked = "(no match)"
			}
			st := totals[v.name]
			st.total++
			if got.Matched && got.ToolID == "f5_product_cases" {
				st.hit++
			}
			totals[v.name] = st

			gate := " "
			if f5 < accept {
				gate = "<"
			}
			fmt.Printf("%-11s %-38s %-18s %-7.4f %s%-6.2f %s\n",
				v.name, p.text, picked, f5, gate, accept, top)
		}
	}

	fmt.Println()
	names := make([]string, 0, len(totals))
	for n := range totals {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		st := totals[n]
		fmt.Printf("%-11s f5 selected %d/%d\n", n, st.hit, st.total)
	}
}

func TestVerticalGeneralisation(t *testing.T) {
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

	_, model, shared, err := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg))
	if err != nil {
		t.Fatalf("embedder: %v", err)
	}

	runVerticals(t, "A  current anchors (robot vocabulary)  "+model, shared, catalog)
	runVerticals(t, "N  f5 anchors domain-neutral  "+model, shared, neutralCatalog(catalog))
}
