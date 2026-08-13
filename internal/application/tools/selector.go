package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/my/app/internal/shared/ctxkey"
	"github.com/my/app/internal/shared/perms"
	"github.com/my/app/internal/shared/vec"
)

const embedRetryDelay = 250 * time.Millisecond

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type Selection struct {
	ToolID        string
	Score         float64
	RunnerUp      float64
	Matched       bool
	Params        map[string]string
	Specificity   int
	Entities      int
	PermFiltered  bool
	MissingParams []string
}

type toolVectors struct {
	id     string
	perm   string
	intent string
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

const defaultFloor = 0.35

var acceptByKind = map[string]float64{
	"read":      0.55,
	"retrieval": 0.55,
	"write":     0.62,
}

func thresholdsFor(t Tool) (float64, float64) {
	accept := t.Thresholds.Accept
	if accept <= 0 {
		if byKind, ok := acceptByKind[t.Kind]; ok {
			accept = byKind
		} else {
			accept = defaultAccept
		}
	}
	floor := t.Thresholds.Floor
	if floor <= 0 {
		floor = defaultFloor
	}
	if floor > accept {
		floor = accept
	}
	return accept, floor
}

const specificityMargin = 0.05

const entityEvidenceWeight = 0.145

const requiredEvidenceWeight = 0.05

const intentEvidenceWeight = 0.05

func preferOver(cand, best Selection) bool {
	switch {
	case !best.Matched:
		return true
	case cand.Entities != best.Entities:
		return cand.Entities > best.Entities
	case cand.Specificity > best.Specificity:
		return cand.Score >= best.Score-specificityMargin
	case cand.Specificity < best.Specificity:
		return cand.Score-specificityMargin > best.Score
	default:
		return cand.Score > best.Score
	}
}

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

		accept, floor := thresholdsFor(t)

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
			intent: t.Intent,
			accept: accept,
			floor:  floor,
			vecs:   normed,
			params: compiled,
		})
	}

	return sel, nil
}

func (s *Selector) Select(ctx context.Context, text string, granted []string) (Selection, error) {
	embedText := normalizeQuery(text)
	var query []float32
	if shared := ctxkey.QueryVector(ctx); len(shared) > 0 && embedText == text {
		query = shared
	} else {
		vecs, err := s.emb.Embed(ctx, []string{embedText})
		if err != nil {
			select {
			case <-ctx.Done():
				return Selection{}, fmt.Errorf("tools: embed query: %w", err)
			case <-time.After(embedRetryDelay):
			}
			var retryErr error
			vecs, retryErr = s.emb.Embed(ctx, []string{embedText})
			if retryErr != nil {
				return Selection{}, fmt.Errorf("tools: embed query: %w (retry: %v)", err, retryErr)
			}
		}
		if len(vecs) != 1 {
			return Selection{}, fmt.Errorf("tools: query embedding count %d != 1", len(vecs))
		}
		query = vec.Normalize(vecs[0])
	}
	if len(s.tools) > 0 && len(s.tools[0].vecs) > 0 && len(query) != len(s.tools[0].vecs[0]) {
		return Selection{}, fmt.Errorf("tools: query dim %d != anchor dim %d", len(query), len(s.tools[0].vecs[0]))
	}

	best := Selection{}
	nearMiss := Selection{}
	needsParams := Selection{}
	routerIntent := ctxkey.RouterIntent(ctx)
	permPassed, permBlocked := 0, 0
	var top1, top2 float64
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
		if t.intent != "" && t.intent == routerIntent {
			score += intentEvidenceWeight
		}

		if score > top1 {
			top1, top2 = score, top1
		} else if score > top2 {
			top2 = score
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
			if score >= t.accept && score > needsParams.Score {
				needsParams = Selection{
					ToolID:        t.id,
					Score:         score,
					Params:        extracted,
					MissingParams: missingRequired(t.params, extracted),
				}
			}
			continue
		}
		specificity := requiredMatched(t.params, extracted)
		entities := entityMatched(t.params, extracted)
		evidence := score +
			entityEvidenceWeight*float64(entities) +
			requiredEvidenceWeight*float64(specificity-entities)
		if evidence < t.accept {
			continue
		}

		cand := Selection{
			ToolID:      t.id,
			Score:       score,
			Matched:     true,
			Params:      extracted,
			Specificity: specificity,
			Entities:    entities,
		}
		if preferOver(cand, best) {
			best = cand
		}
	}

	runnerUp := 0.0
	if permPassed >= 2 {
		runnerUp = top2
	}

	if !best.Matched {
		if needsParams.ToolID != "" {
			needsParams.PermFiltered = permBlocked > 0
			needsParams.RunnerUp = runnerUp
			return needsParams, nil
		}
		nearMiss.PermFiltered = permBlocked > 0
		nearMiss.RunnerUp = runnerUp
		return nearMiss, nil
	}

	best.RunnerUp = runnerUp
	return best, nil
}
