package tools

import (
	"context"
	"fmt"
	vecmath "github.com/my/app/internal/shared/vec"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

func TestProbeScoreSpread(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("set PROBE=1")
	}
	cfg, err := config.LoadFrom("/app/config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	_, model, emb, err := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg))
	if err != nil {
		t.Fatalf("embedder: %v", err)
	}
	catalog, err := Load(os.DirFS("/app/config/tools"), nil)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	ctx := context.Background()
	sel, err := NewSelector(ctx, emb, catalog)
	if err != nil {
		t.Fatalf("selector: %v", err)
	}
	fmt.Printf("model=%s tools=%d\n\n", model, len(sel.tools))

	probes := []string{
		"test",
		"ok",
		"aaa",
		"ทดสอบ",
		"Bella Bot",
		"cases for Bella Bot",
		"show me cases for product Bella Bot",
		"เคสของสินค้า Flash",
		"check case status of BOT-1234",
		"show latest cases",
		"what is the weather tomorrow",
		"give me mail intake data list",
		"approve the Sophie Mendel",
		"approve it",
		"promote the Sophie Mendel email to a case",
		"create a case from the LinkedIn email",
		"ยืนยัน",
		"อนุมัติเมลของ Sophie Mendel",
	}

	for _, p := range probes {
		vecs, err := emb.Embed(ctx, []string{p})
		if err != nil {
			t.Fatalf("embed %q: %v", p, err)
		}
		q := vecmath.Normalize(vecs[0])

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
		sort.Slice(all, func(i, j int) bool { return all[i].s > all[j].s })

		sum, sumsq := 0.0, 0.0
		for _, a := range all {
			sum += a.s
			sumsq += a.s * a.s
		}
		n := float64(len(all))
		mean := sum / n
		sd := math.Sqrt(sumsq/n - mean*mean)
		ent := 0.0
		for _, a := range all {
			pi := a.s / sum
			if pi > 0 {
				ent -= pi * math.Log(pi)
			}
		}
		ent /= math.Log(n)

		top1, top2 := all[0].s, all[1].s
		z := 0.0
		if sd > 0 {
			z = (top1 - mean) / sd
		}
		fired := "-"
		for _, tv := range sel.tools {
			if tv.id == all[0].id && top1 >= tv.accept {
				fired = "FIRES"
			}
		}
		fmt.Printf("%-38q %s\n", p, fired)
		fmt.Printf("   top1=%.4f(%s) top2=%.4f(%s) gap=%.4f mean=%.4f sd=%.4f z=%.2f H=%.4f\n",
			top1, all[0].id, top2, all[1].id, top1-top2, mean, sd, z, ent)
		for _, a := range all {
			fmt.Printf("      %-22s %.4f\n", a.id, a.s)
		}
		fmt.Println()
	}
}
