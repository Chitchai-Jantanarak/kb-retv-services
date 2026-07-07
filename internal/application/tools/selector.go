package tools

import (
	"context"
	"fmt"
	"math"

	"github.com/my/app/internal/shared/perms"
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type Selection struct {
	ToolID  string
	Score   float64
	Matched bool
}

type toolVectors struct {
	id     string
	perm   string
	accept float64
	vecs   [][]float32
}

type Selector struct {
	emb   Embedder
	tools []toolVectors
}

const defaultAccept = 0.55

func NewSelector(ctx context.Context, emb Embedder, ts []Tool) (*Selector, error) {
	if emb == nil {
		return nil, fmt.Errorf("tools: embedder is required")
	}

	sel := &Selector{emb: emb}
	for _, t := range ts {
		if len(t.Anchors) == 0 {
			continue
		}

		vecs, err := emb.Embed(ctx, t.Anchors)
		if err != nil {
			return nil, fmt.Errorf("tools: embed anchors for %s: %w", t.ID, err)
		}

		accept := t.Thresholds.Accept
		if accept <= 0 {
			accept = defaultAccept
		}

		normed := make([][]float32, len(vecs))
		for i, v := range vecs {
			normed[i] = normalize(v)
		}

		sel.tools = append(sel.tools, toolVectors{
			id:     t.ID,
			perm:   t.RBAC.RequiresPermission,
			accept: accept,
			vecs:   normed,
		})
	}

	return sel, nil
}

func (s *Selector) Select(ctx context.Context, text string, granted []string) (Selection, error) {
	vecs, err := s.emb.Embed(ctx, []string{text})
	if err != nil {
		return Selection{}, fmt.Errorf("tools: embed query: %w", err)
	}
	if len(vecs) != 1 {
		return Selection{}, fmt.Errorf("tools: query embedding count %d != 1", len(vecs))
	}
	query := normalize(vecs[0])

	best := Selection{}
	for _, t := range s.tools {
		if !perms.Can(granted, t.perm) {
			continue
		}

		score := 0.0
		for _, v := range t.vecs {
			if d := dot(query, v); d > score {
				score = d
			}
		}

		if score > best.Score {
			best = Selection{ToolID: t.id, Score: score, Matched: score >= t.accept}
		}
	}

	return best, nil
}

func normalize(vec []float32) []float32 {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return vec
	}
	norm := math.Sqrt(sum)
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = float32(float64(v) / norm)
	}
	return out
}

func dot(a, b []float32) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var sum float64
	for i := 0; i < n; i++ {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}
