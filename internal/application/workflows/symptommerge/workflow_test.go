package symptommerge

import (
	"context"
	"testing"
)

type fakeStore struct {
	rows   []SymptomRow
	merged map[int64]int64
}

func (f *fakeStore) UnmergedSymptoms(context.Context, int64) ([]SymptomRow, error) {
	return f.rows, nil
}

func (f *fakeStore) SetCanonical(_ context.Context, _ int64, id, canonicalID int64) error {
	f.merged[id] = canonicalID
	return nil
}

type fakeEmbedder struct{ byName map[string][]float32 }

func (f fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = f.byName[t]
	}
	return out, nil
}

func TestRunMergesSimilarIntoMostUsed(t *testing.T) {
	store := &fakeStore{
		rows: []SymptomRow{
			{ID: 1, Name: "th-no-power", Count: 163},
			{ID: 2, Name: "no-power", Count: 158},
			{ID: 3, Name: "nav-blocked", Count: 138},
			{ID: 4, Name: "th-nav-blocked", Count: 87},
		},
		merged: map[int64]int64{},
	}
	emb := fakeEmbedder{byName: map[string][]float32{
		"th-no-power":    {1, 0, 0},
		"no-power":       {0.99, 0.14, 0},
		"nav-blocked":    {0, 1, 0},
		"th-nav-blocked": {0.1, 0.99, 0},
	}}
	w, err := New(Config{Store: store, Embedder: emb, Threshold: 0.85})
	if err != nil {
		t.Fatal(err)
	}
	res, err := w.Run(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if res.Examined != 4 || res.Canonical != 2 || res.Merged != 2 {
		t.Fatalf("res = %+v, want examined 4 canonical 2 merged 2", res)
	}
	if store.merged[2] != 1 {
		t.Fatalf("no-power should merge into th-no-power (higher count): %v", store.merged)
	}
	if store.merged[4] != 3 {
		t.Fatalf("th-nav-blocked should merge into nav-blocked: %v", store.merged)
	}
}

func TestRunDissimilarStaysCanonical(t *testing.T) {
	store := &fakeStore{
		rows:   []SymptomRow{{ID: 1, Name: "a", Count: 5}, {ID: 2, Name: "b", Count: 3}},
		merged: map[int64]int64{},
	}
	emb := fakeEmbedder{byName: map[string][]float32{"a": {1, 0, 0}, "b": {0, 1, 0}}}
	w, _ := New(Config{Store: store, Embedder: emb, Threshold: 0.85})
	res, err := w.Run(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if res.Merged != 0 || res.Canonical != 2 {
		t.Fatalf("res = %+v, want no merges", res)
	}
}
