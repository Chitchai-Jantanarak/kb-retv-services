package embeddings

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxVocab = 1 << 24
	maxDim   = 4096
)

func validateThresholds(meta staticMeta) error {
	if meta.Floor <= 0 || meta.Floor >= 1 {
		return fmt.Errorf("guard embedder: floor %v out of range (0,1)", meta.Floor)
	}
	if meta.Accept <= meta.Floor || meta.Accept > 1 {
		return fmt.Errorf("guard embedder: accept %v must be in (floor,1]", meta.Accept)
	}
	if meta.Margin <= 0 || meta.Margin >= 1 {
		return fmt.Errorf("guard embedder: margin %v out of range (0,1)", meta.Margin)
	}
	return nil
}

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
	Vocab      int               `json:"vocab"`
	Dim        int               `json:"dim"`
	UnkID      int               `json:"unk_id"`
	Floor      float64           `json:"floor"`
	Accept     float64           `json:"accept"`
	Margin     float64           `json:"margin"`
	Calibrated bool              `json:"calibrated"`
	Files      map[string]string `json:"files,omitempty"`
}

func verifyRecordedFileDigests(dir string, files map[string]string) error {
	for name, want := range files {
		if strings.ContainsAny(name, `/\`) {
			return fmt.Errorf("guard embedder: files entry %q must be a bare filename", name)
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("guard embedder: hash %s: %w", name, err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(raw)); !strings.EqualFold(got, want) {
			return fmt.Errorf("guard embedder: %s does not match the digest recorded at calibration", name)
		}
	}
	return nil
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
	if meta.Vocab <= 0 || meta.Vocab > maxVocab || meta.Dim <= 0 || meta.Dim > maxDim {
		return nil, fmt.Errorf("guard embedder: bad meta vocab=%d dim=%d", meta.Vocab, meta.Dim)
	}
	if meta.UnkID < 0 || meta.UnkID >= meta.Vocab {
		return nil, fmt.Errorf("guard embedder: unk_id %d out of range [0,%d)", meta.UnkID, meta.Vocab)
	}
	if !meta.Calibrated {
		return nil, fmt.Errorf("guard embedder: asset not calibrated; run guard_embedder.verify after build")
	}
	if err := validateThresholds(meta); err != nil {
		return nil, err
	}
	if err := verifyRecordedFileDigests(dir, meta.Files); err != nil {
		return nil, err
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
	for i, s := range scales.PerRow {
		if s <= 0 || float64(s) > math.MaxFloat32/127 || math.IsInf(float64(s), 0) || math.IsNaN(float64(s)) {
			return nil, fmt.Errorf("guard embedder: scale %d is not a usable positive value", i)
		}
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
