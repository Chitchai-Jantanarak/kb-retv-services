package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/my/app/internal/shared/perms"
	"github.com/my/app/internal/shared/vec"
)

const embedRetryDelay = 250 * time.Millisecond

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type Selection struct {
	ToolID       string
	Score        float64
	Matched      bool
	Params       map[string]string
	Specificity  int
	PermFiltered bool
}

type toolVectors struct {
	id     string
	perm   string
	accept float64
	floor  float64
	vecs   [][]float32
	params []compiledParam
}

type Selector struct {
	emb   Embedder
	names NameSource
	tools []toolVectors
}

type SelectorOption func(*Selector)

func WithNameSource(names NameSource) SelectorOption {
	return func(s *Selector) { s.names = names }
}

const defaultAccept = 0.55

func NewSelector(ctx context.Context, emb Embedder, ts []Tool, opts ...SelectorOption) (*Selector, error) {
	if emb == nil {
		return nil, fmt.Errorf("tools: embedder is required")
	}

	sel := &Selector{emb: emb}
	for _, opt := range opts {
		opt(sel)
	}
	for _, t := range ts {
		if len(t.Anchors) == 0 {
			continue
		}

		vecs, err := emb.Embed(ctx, t.Anchors)
		if err != nil {
			return nil, fmt.Errorf("tools: embed anchors for %s: %w", t.ID, err)
		}
		if len(vecs) != len(t.Anchors) {
			return nil, fmt.Errorf("tools: %s: anchor embedding count %d != %d", t.ID, len(vecs), len(t.Anchors))
		}

		accept := t.Thresholds.Accept
		if accept <= 0 {
			accept = defaultAccept
		}
		floor := t.Thresholds.Floor
		if floor <= 0 || floor > accept {
			floor = accept
		}

		normed := make([][]float32, len(vecs))
		for i, v := range vecs {
			normed[i] = vec.Normalize(v)
		}

		compiled, err := compileParams(t.Params)
		if err != nil {
			return nil, fmt.Errorf("tools: %s: %w", t.ID, err)
		}

		sel.tools = append(sel.tools, toolVectors{
			id:     t.ID,
			perm:   t.RBAC.RequiresPermission,
			accept: accept,
			floor:  floor,
			vecs:   normed,
			params: compiled,
		})
	}

	return sel, nil
}

func (s *Selector) Select(ctx context.Context, text string, granted []string) (Selection, error) {
	vecs, err := s.emb.Embed(ctx, []string{text})
	if err != nil {
		select {
		case <-ctx.Done():
			return Selection{}, fmt.Errorf("tools: embed query: %w", err)
		case <-time.After(embedRetryDelay):
		}
		var retryErr error
		vecs, retryErr = s.emb.Embed(ctx, []string{text})
		if retryErr != nil {
			return Selection{}, fmt.Errorf("tools: embed query: %w (retry: %v)", err, retryErr)
		}
	}
	if len(vecs) != 1 {
		return Selection{}, fmt.Errorf("tools: query embedding count %d != 1", len(vecs))
	}
	query := vec.Normalize(vecs[0])
	if len(s.tools) > 0 && len(s.tools[0].vecs) > 0 && len(query) != len(s.tools[0].vecs[0]) {
		return Selection{}, fmt.Errorf("tools: query dim %d != anchor dim %d", len(query), len(s.tools[0].vecs[0]))
	}

	best := Selection{}
	nearMiss := Selection{}
	permPassed, permBlocked := 0, 0
	for _, t := range s.tools {
		if !perms.Can(granted, t.perm) {
			permBlocked++
			continue
		}
		permPassed++

		score := 0.0
		for _, v := range t.vecs {
			if d := vec.Dot(query, v); d > score {
				score = d
			}
		}

		if score > nearMiss.Score {
			nearMiss = Selection{ToolID: t.id, Score: score}
		}

		if score < t.floor {
			continue
		}

		extracted, satisfied, err := s.extractParams(ctx, t.params, text)
		if err != nil {
			return Selection{}, err
		}
		if !satisfied {
			continue
		}
		specificity := requiredMatched(t.params, extracted)

		corroborated := requiredCount(t.params) > 0
		if !corroborated && score < t.accept {
			continue
		}

		if !best.Matched || specificity > best.Specificity ||
			(specificity == best.Specificity && score > best.Score) {
			best = Selection{ToolID: t.id, Score: score, Matched: true, Params: extracted, Specificity: specificity}
		}
	}

	if !best.Matched {
		nearMiss.PermFiltered = permBlocked > 0
		return nearMiss, nil
	}

	return best, nil
}
