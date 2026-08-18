package symptommerge

import (
	"context"
	"errors"
	"fmt"

	vecmath "github.com/my/app/internal/shared/vec"
)

type SymptomRow struct {
	ID    int64
	Name  string
	Count int64
}

type Store interface {
	UnmergedSymptoms(ctx context.Context, companyID int64) ([]SymptomRow, error)
	SetCanonical(ctx context.Context, companyID, id, canonicalID int64) error
}

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type Config struct {
	Store     Store
	Embedder  Embedder
	Threshold float64
}

type Result struct {
	Examined  int
	Canonical int
	Merged    int
}

type Workflow struct {
	store     Store
	embedder  Embedder
	threshold float64
}

func New(cfg Config) (*Workflow, error) {
	if cfg.Store == nil || cfg.Embedder == nil {
		return nil, errors.New("symptommerge: store and embedder are required")
	}
	th := cfg.Threshold
	if th <= 0 || th >= 1 {
		th = 0.85
	}
	return &Workflow{store: cfg.Store, embedder: cfg.Embedder, threshold: th}, nil
}

func (w *Workflow) Run(ctx context.Context, companyID int64) (Result, error) {
	if companyID <= 0 {
		return Result{}, errors.New("symptommerge: company_id must be positive")
	}
	rows, err := w.store.UnmergedSymptoms(ctx, companyID)
	if err != nil {
		return Result{}, err
	}
	res := Result{Examined: len(rows)}
	if len(rows) < 2 {
		res.Canonical = len(rows)
		return res, nil
	}

	vecs := make([][]float32, 0, len(rows))
	for start := 0; start < len(rows); start += 100 {
		end := min(start+100, len(rows))
		texts := make([]string, 0, end-start)
		for _, r := range rows[start:end] {
			texts = append(texts, r.Name)
		}
		batch, err := w.embedder.Embed(ctx, texts)
		if err != nil {
			return res, fmt.Errorf("symptommerge: embed [%d:%d]: %w", start, end, err)
		}
		vecs = append(vecs, batch...)
	}
	if len(vecs) != len(rows) {
		return res, fmt.Errorf("symptommerge: embed count %d != %d", len(vecs), len(rows))
	}
	for i := range vecs {
		vecs[i] = vecmath.Normalize(vecs[i])
	}

	// rows arrive count-desc: the most-used name of each cluster becomes canonical
	canonIdx := make([]int, 0, len(rows))
	for i, row := range rows {
		bestIdx, bestCos := -1, 0.0
		for _, c := range canonIdx {
			if d := vecmath.Dot(vecs[i], vecs[c]); d > bestCos {
				bestCos, bestIdx = d, c
			}
		}
		if bestIdx >= 0 && bestCos >= w.threshold {
			if err := w.store.SetCanonical(ctx, companyID, row.ID, rows[bestIdx].ID); err != nil {
				return res, fmt.Errorf("symptommerge: set canonical %d->%d: %w", row.ID, rows[bestIdx].ID, err)
			}
			res.Merged++
			continue
		}
		canonIdx = append(canonIdx, i)
	}
	res.Canonical = len(canonIdx)
	return res, nil
}
