package intent

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/my/app/internal/shared/vec"
)

type Intent string

const (
	KBSearch       Intent = "kb_search"
	CaseStatus     Intent = "case_status"
	Handoff        Intent = "handoff"
	OpenCase       Intent = "open_case"
	GeneralSupport Intent = "general_support"
	OffDomain      Intent = "off_domain"
	Social         Intent = "social"
)

type Reason string

const (
	ReasonActionAnchor    Reason = "action_anchor"
	ReasonRetrievalHit    Reason = "retrieval_hit"
	ReasonInDomainNoKB    Reason = "in_domain_no_kb"
	ReasonOffDomainFloor  Reason = "off_domain_floor"
	ReasonOffDomainAnchor Reason = "off_domain_anchor"
	ReasonSocialAnchor    Reason = "social_anchor"
)

const (
	defaultFloor  = 0.35
	defaultAccept = 0.55
	defaultMargin = 0.05
)

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type RetrievalScorer interface {
	TopScore(ctx context.Context, query string) (float64, error)
}

type Decision struct {
	Intent         Intent
	Reason         Reason
	Confidence     float64
	RetrievalScore float64
}

type anchor struct {
	intent Intent
	vec    []float32
}

type Router struct {
	emb     Embedder
	scorer  RetrievalScorer
	anchors []anchor
	floor   float64
	accept  float64
	margin  float64
}

type config struct {
	anchors map[Intent][]string
	floor   float64
	accept  float64
	margin  float64
}

type Option func(*config)

func WithAnchors(anchors map[Intent][]string) Option {
	return func(c *config) {
		if len(anchors) > 0 {
			c.anchors = anchors
		}
	}
}

func WithThresholds(floor, accept float64) Option {
	return func(c *config) {
		c.floor = floor
		c.accept = accept
	}
}

func WithMargin(margin float64) Option {
	return func(c *config) {
		c.margin = margin
	}
}

func New(ctx context.Context, emb Embedder, scorer RetrievalScorer, opts ...Option) (*Router, error) {
	if emb == nil {
		return nil, fmt.Errorf("intent: embedder is required")
	}
	if scorer == nil {
		return nil, fmt.Errorf("intent: retrieval scorer is required")
	}

	defaultAnchors, err := loadAnchors(anchorsJSON)
	if err != nil {
		return nil, err
	}
	cfg := config{
		anchors: defaultAnchors,
		floor:   defaultFloor,
		accept:  defaultAccept,
		margin:  defaultMargin,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	phrases, owners := flatten(cfg.anchors)
	if len(phrases) == 0 {
		return nil, fmt.Errorf("intent: at least one anchor is required")
	}
	if len(cfg.anchors[Social]) == 0 {
		return nil, fmt.Errorf("intent: social anchors are required; without them neutral messages fall through to the tool layer")
	}
	if len(cfg.anchors[OffDomain]) == 0 {
		return nil, fmt.Errorf("intent: off_domain anchors are required; without them the negative guard silently passes everything")
	}

	vecs, err := emb.Embed(ctx, phrases)
	if err != nil {
		return nil, fmt.Errorf("intent: embed anchors: %w", err)
	}
	if len(vecs) != len(phrases) {
		return nil, fmt.Errorf("intent: anchor embedding count %d != %d", len(vecs), len(phrases))
	}

	anchors := make([]anchor, len(phrases))
	for i := range phrases {
		anchors[i] = anchor{intent: owners[i], vec: vec.Normalize(vecs[i])}
	}
	return &Router{
		emb:     emb,
		scorer:  scorer,
		anchors: anchors,
		floor:   cfg.floor,
		accept:  cfg.accept,
		margin:  cfg.margin,
	}, nil
}

func (r *Router) Route(ctx context.Context, text string) (Decision, error) {
	vecs, err := r.emb.Embed(ctx, []string{text})
	if err != nil {
		return Decision{}, fmt.Errorf("intent: embed query: %w", err)
	}
	if len(vecs) != 1 {
		return Decision{}, fmt.Errorf("intent: query embedding count %d != 1", len(vecs))
	}
	query := vec.Normalize(vecs[0])

	pos, neg := r.scoreAnchors(query)
	if neg >= pos.score+r.margin {
		return Decision{Intent: OffDomain, Reason: ReasonOffDomainAnchor, Confidence: pos.score}, nil
	}

	if pos.intent == Social && pos.score >= r.accept {
		return Decision{Intent: Social, Reason: ReasonSocialAnchor, Confidence: pos.score}, nil
	}

	if isAction(pos.intent) && pos.score >= r.accept {
		return Decision{Intent: pos.intent, Reason: ReasonActionAnchor, Confidence: pos.score}, nil
	}

	score, err := r.scorer.TopScore(ctx, text)
	if err != nil {
		return Decision{}, fmt.Errorf("intent: retrieval score: %w", err)
	}

	switch {
	case score > 0:
		return Decision{Intent: KBSearch, Reason: ReasonRetrievalHit, Confidence: pos.score, RetrievalScore: score}, nil
	case pos.score >= r.floor:
		return Decision{Intent: GeneralSupport, Reason: ReasonInDomainNoKB, Confidence: pos.score, RetrievalScore: score}, nil
	default:
		return Decision{Intent: OffDomain, Reason: ReasonOffDomainFloor, Confidence: pos.score, RetrievalScore: score}, nil
	}
}

type scored struct {
	intent Intent
	score  float64
}

func (r *Router) scoreAnchors(query []float32) (scored, float64) {
	best := make(map[Intent]float64, len(r.anchors))
	for _, a := range r.anchors {
		s := vec.Dot(query, a.vec)
		if cur, ok := best[a.intent]; !ok || s > cur {
			best[a.intent] = s
		}
	}

	pos := scored{}
	found := false
	for intent, score := range best {
		if intent == OffDomain {
			continue
		}
		if !found || score > pos.score || (score == pos.score && intent < pos.intent) {
			pos = scored{intent: intent, score: score}
			found = true
		}
	}
	return pos, best[OffDomain]
}

func isAction(i Intent) bool {
	switch i {
	case Handoff, CaseStatus, OpenCase:
		return true
	default:
		return false
	}
}

func flatten(anchors map[Intent][]string) ([]string, []Intent) {
	intents := make([]Intent, 0, len(anchors))
	for intent := range anchors {
		intents = append(intents, intent)
	}
	slices.Sort(intents)

	var (
		phrases []string
		owners  []Intent
	)
	for _, intent := range intents {
		for _, phrase := range anchors[intent] {
			phrases = append(phrases, phrase)
			owners = append(owners, intent)
		}
	}
	return phrases, owners
}

//go:embed anchors.json
var anchorsJSON []byte

func loadAnchors(raw []byte) (map[Intent][]string, error) {
	var anchors map[Intent][]string
	if err := json.Unmarshal(raw, &anchors); err != nil {
		return nil, fmt.Errorf("intent: invalid anchors.json: %w", err)
	}
	return anchors, nil
}
