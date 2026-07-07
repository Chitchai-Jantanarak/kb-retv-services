package embeddings

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type stubEncoder struct{ ids map[string][]uint32 }

func (s stubEncoder) EncodeIDs(text string) []uint32 { return s.ids[text] }

func writeFixtureTable(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "meta.json"),
		[]byte(`{"vocab":3,"dim":2,"unk_id":0,"floor":0.35,"accept":0.55,"margin":0.08,"calibrated":true}`), 0o644))
	must(os.WriteFile(filepath.Join(dir, "model.int8.bin"),
		[]byte{0, 0, 127, 0, 0, 127}, 0o644))
	scales, _ := json.Marshal(map[string][]float32{"per_row": {1, 0.01, 0.01}})
	must(os.WriteFile(filepath.Join(dir, "scales.json"), scales, 0o644))
	return dir
}

func TestLoadStaticTableRowAndThresholds(t *testing.T) {
	t.Parallel()
	tb, err := loadStaticTable(writeFixtureTable(t))
	if err != nil {
		t.Fatalf("loadStaticTable: %v", err)
	}
	if tb.vocab != 3 || tb.dim != 2 {
		t.Fatalf("vocab/dim = %d/%d, want 3/2", tb.vocab, tb.dim)
	}
	if tb.margin != 0.08 || tb.floor != 0.35 {
		t.Fatalf("thresholds not loaded: floor=%v margin=%v", tb.floor, tb.margin)
	}
	got := tb.row(1)
	if len(got) != 2 || got[0] != 127*0.01 || got[1] != 0 {
		t.Fatalf("row(1) = %v, want [1.27 0]", got)
	}
	if oob := tb.row(99); oob[0] != tb.row(0)[0] {
		t.Fatalf("out-of-range id must fall back to unk row")
	}
}

func TestEmbedIDsMeanPoolMatchesFixtures(t *testing.T) {
	t.Parallel()
	assetDir := os.Getenv("GUARD_ASSET_DIR")
	if assetDir == "" {
		assetDir = filepath.Join("..", "..", "..", "assets", "guard-embedder")
	}
	tb, err := loadStaticTable(assetDir)
	if err != nil {
		t.Skipf("guard asset unavailable: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(assetDir, "fixtures.json"))
	if err != nil {
		t.Skipf("fixtures unavailable: %v", err)
	}
	var fx []struct {
		Text     string    `json:"text"`
		TokenIDs []uint32  `json:"token_ids"`
		Vec128   []float32 `json:"vec128"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}
	p := &StaticLocalProvider{table: tb}
	for _, f := range fx {
		got := p.embedIDs(f.TokenIDs)
		if len(got) != len(f.Vec128) {
			t.Fatalf("%q: dim %d != %d", f.Text, len(got), len(f.Vec128))
		}
		for i := range got {
			if math.Abs(float64(got[i]-f.Vec128[i])) > 1e-4 {
				t.Fatalf("%q dim %d: Go %v vs Py %v (parity drift)", f.Text, i, got[i], f.Vec128[i])
			}
		}
	}
}

func TestEmbedIDsEmptyFallsBackToUnk(t *testing.T) {
	t.Parallel()
	tb, err := loadStaticTable(writeFixtureTable(t))
	if err != nil {
		t.Fatal(err)
	}
	p := &StaticLocalProvider{table: tb}
	if v := p.embedIDs(nil); len(v) != tb.dim {
		t.Fatalf("empty ids must still yield dim-length vector, got %d", len(v))
	}
}

func TestLoadStaticTableRejectsBadUnkID(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "meta.json"),
		[]byte(`{"vocab":2,"dim":2,"unk_id":5}`), 0o644))
	must(os.WriteFile(filepath.Join(dir, "model.int8.bin"), []byte{0, 0, 1, 1}, 0o644))
	scales, _ := json.Marshal(map[string][]float32{"per_row": {1, 1}})
	must(os.WriteFile(filepath.Join(dir, "scales.json"), scales, 0o644))
	if _, err := loadStaticTable(dir); err == nil {
		t.Fatal("loadStaticTable must reject unk_id out of [0,vocab)")
	}
}

func TestLoadStaticTableRejectsUncalibrated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "meta.json"),
		[]byte(`{"vocab":3,"dim":2,"unk_id":0,"floor":0.35,"accept":0.55,"margin":0.08,"calibrated":false}`), 0o644))
	must(os.WriteFile(filepath.Join(dir, "model.int8.bin"), []byte{0, 0, 127, 0, 0, 127}, 0o644))
	scales, _ := json.Marshal(map[string][]float32{"per_row": {1, 0.01, 0.01}})
	must(os.WriteFile(filepath.Join(dir, "scales.json"), scales, 0o644))
	if _, err := loadStaticTable(dir); err == nil {
		t.Fatal("loadStaticTable must reject an uncalibrated asset (build ran, verify did not)")
	}
}

func TestStaticLocalProviderEmbedDimThresholds(t *testing.T) {
	t.Parallel()
	tb, err := loadStaticTable(writeFixtureTable(t))
	if err != nil {
		t.Fatal(err)
	}
	p := &StaticLocalProvider{table: tb, enc: stubEncoder{ids: map[string][]uint32{"a": {1}}}}

	if p.Dim() != tb.dim {
		t.Fatalf("Dim = %d, want %d", p.Dim(), tb.dim)
	}
	if f, a, m := p.Thresholds(); f != 0.35 || a != 0.55 || m != 0.08 {
		t.Fatalf("Thresholds = %v/%v/%v, want 0.35/0.55/0.08", f, a, m)
	}

	out, err := p.Embed(context.Background(), []string{"a"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(out) != 1 || len(out[0]) != tb.dim {
		t.Fatalf("Embed shape = %dx%d, want 1x%d", len(out), len(out[0]), tb.dim)
	}
	if math.Abs(float64(out[0][0])-1) > 1e-6 || out[0][1] != 0 {
		t.Fatalf("Embed(\"a\") = %v, want [1 0] after normalize", out[0])
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Embed(cancelled, []string{"a"}); err == nil {
		t.Fatal("Embed must respect a cancelled context")
	}
}
