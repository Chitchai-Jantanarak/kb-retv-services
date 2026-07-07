package embeddings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type staticTable struct {
	vocab  int
	dim    int
	unkID  int
	rows   []int8
	scales []float32
	floor  float64
	accept float64
	margin float64
}

type staticMeta struct {
	Vocab      int     `json:"vocab"`
	Dim        int     `json:"dim"`
	UnkID      int     `json:"unk_id"`
	Floor      float64 `json:"floor"`
	Accept     float64 `json:"accept"`
	Margin     float64 `json:"margin"`
	Calibrated bool    `json:"calibrated"`
}

func loadStaticTable(dir string) (*staticTable, error) {
	metaRaw, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return nil, fmt.Errorf("guard embedder: read meta: %w", err)
	}
	var meta staticMeta
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return nil, fmt.Errorf("guard embedder: parse meta: %w", err)
	}
	if meta.Vocab <= 0 || meta.Dim <= 0 {
		return nil, fmt.Errorf("guard embedder: bad meta vocab=%d dim=%d", meta.Vocab, meta.Dim)
	}
	if meta.UnkID < 0 || meta.UnkID >= meta.Vocab {
		return nil, fmt.Errorf("guard embedder: unk_id %d out of range [0,%d)", meta.UnkID, meta.Vocab)
	}

	rowsRaw, err := os.ReadFile(filepath.Join(dir, "model.int8.bin"))
	if err != nil {
		return nil, fmt.Errorf("guard embedder: read table: %w", err)
	}
	if want := meta.Vocab * meta.Dim; len(rowsRaw) != want {
		return nil, fmt.Errorf("guard embedder: table size %d != %d", len(rowsRaw), want)
	}
	rows := make([]int8, len(rowsRaw))
	for i, b := range rowsRaw {
		rows[i] = int8(b)
	}

	scalesRaw, err := os.ReadFile(filepath.Join(dir, "scales.json"))
	if err != nil {
		return nil, fmt.Errorf("guard embedder: read scales: %w", err)
	}
	var scales struct {
		PerRow []float32 `json:"per_row"`
	}
	if err := json.Unmarshal(scalesRaw, &scales); err != nil {
		return nil, fmt.Errorf("guard embedder: parse scales: %w", err)
	}
	if len(scales.PerRow) != meta.Vocab {
		return nil, fmt.Errorf("guard embedder: scales len %d != vocab %d", len(scales.PerRow), meta.Vocab)
	}
	if !meta.Calibrated {
		return nil, fmt.Errorf("guard embedder: asset not calibrated; run guard_embedder.verify after build")
	}

	return &staticTable{
		vocab:  meta.Vocab,
		dim:    meta.Dim,
		unkID:  meta.UnkID,
		rows:   rows,
		scales: scales.PerRow,
		floor:  meta.Floor,
		accept: meta.Accept,
		margin: meta.Margin,
	}, nil
}

func (t *staticTable) row(id int) []float32 {
	if id < 0 || id >= t.vocab {
		id = t.unkID
	}
	scale := t.scales[id]
	start := id * t.dim
	out := make([]float32, t.dim)
	for i := 0; i < t.dim; i++ {
		out[i] = float32(t.rows[start+i]) * scale
	}
	return out
}
