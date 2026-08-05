package tools

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

var langOrder = []string{"en", "th", "ja", "ko", "ru", "zh", "vi", "es"}

func shortID(id string) string {
	switch id {
	case "open_case":
		return "open"
	case offDomainToolID:
		return "OFF"
	}
	if i := strings.Index(id, "_"); i > 0 {
		return id[:i]
	}
	return id
}

func dumpMatrix(t *testing.T, label string, sel *Selector, vecs map[string][]float32) {
	t.Helper()

	wants := []string{}
	seen := map[string]bool{}
	engText := map[string]string{}
	for _, p := range langProbes {
		if p.want == "" {
			continue
		}
		if !seen[p.want] {
			seen[p.want] = true
			wants = append(wants, p.want)
		}
		if p.lang == "en" {
			engText[p.want] = p.text
		}
	}
	sort.Strings(wants)

	cell := map[string]map[string]string{}
	for _, p := range langProbes {
		if p.want == "" {
			continue
		}
		r := rankAll(sel, vecs[p.text])
		if cell[p.want] == nil {
			cell[p.want] = map[string]string{}
		}
		if r[0].id == p.want {
			cell[p.want][p.lang] = "ok"
		} else {
			cell[p.want][p.lang] = shortID(r[0].id)
		}
	}

	fmt.Printf("\n================ MATRIX: %s ================\n", label)
	fmt.Printf("%-22s | %s\n", "tool (english probe)", strings.Join(langOrder, "     "))
	fmt.Println(strings.Repeat("-", 78))
	for _, w := range wants {
		row := make([]string, 0, len(langOrder))
		hits := 0
		for _, l := range langOrder {
			v := cell[w][l]
			if v == "ok" {
				hits++
			}
			row = append(row, fmt.Sprintf("%-5s", v))
		}
		fmt.Printf("%-22s | %s  %d/8\n", shortID(w), strings.Join(row, " "), hits)
		fmt.Printf("%-22s   \"%s\"\n", "", engText[w])
	}

	fmt.Printf("\nnot probed as a target: ")
	all := []string{}
	for _, tv := range sel.tools {
		if !seen[tv.id] {
			all = append(all, tv.id)
		}
	}
	sort.Strings(all)
	fmt.Printf("%s\n", strings.Join(all, ", "))
}

func TestProbeMatrix(t *testing.T) {
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
	dumpMatrix(t, "SELECTOR "+model, gsel, gvecs)

	guard, _, _, _ := llmboot.GuardEmbedder(cfg, nil)
	if guard == nil {
		return
	}
	pvecs := embedAllProbes(ctx, t, guard)
	psel, err := NewSelector(ctx, guard, catalog)
	if err != nil {
		t.Fatalf("potion selector: %v", err)
	}
	dumpMatrix(t, "GUARD potion", psel, pvecs)
}
