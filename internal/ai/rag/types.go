package rag

import (
	"context"
	"time"

	"github.com/my/app/internal/domain/ports"
)

type Decision string

const (
	DecisionAuto     Decision = "auto"     // confidence >= threshold: send automatically
	DecisionDraft    Decision = "draft"    // 0.60 <= confidence < threshold: queue for agent review
	DecisionEscalate Decision = "escalate" // confidence < 0.60: hand off; log knowledge gap
)

const (
	ModeFullReview = "full_review"
	ModeFastDraft  = "fast_draft"
	ModeDebug      = "debug"
)

type Query struct {
	CompanyID int64
	Coverage  []int64
	Text      string
	Limit     int
	Mode      string
	Debug     bool
	Metadata  map[string]string
}

type Meta struct {
	Terms []string
}

type PlannedQuery struct {
	Kind string
	Text string
}

const PlannedQueryOriginal = "original"

type GraphAnchor struct {
	Kind string
	ID   int64
}

const GraphAnchorSymptom = "symptom"

type QueryPlan struct {
	OriginalText     string
	RetrievalText    string
	CanonicalText    string
	Meta             Meta
	EffectiveLimit   int
	IsNavigational   bool
	RetrievalQueries []PlannedQuery
	GraphAnchors     []GraphAnchor
	SkipReasons      map[string]string
}

type Candidate struct {
	ID            string
	ArticleID     string
	ChunkID       string
	ParentChunkID string
	Title         string
	Content       string
	Source        string
	Score         float64
}

type Result struct {
	Meta           Meta
	Candidates     []Candidate
	Context        string
	CacheHit       bool
	Confidence     float64
	Decision       Decision
	Reason         Reason
	RetryAfterMs   int64
	Draft          string
	CRAG           CRAGResult
	Critique       CritiqueResult
	StageTimingsMS map[string]int64
	DebugTrace     *DebugTrace
}

type DebugTrace struct {
	Mode      string              `json:"mode,omitempty"`
	Plan      DebugPlanTrace      `json:"plan"`
	Retrieval DebugRetrievalTrace `json:"retrieval"`
	Quality   DebugQualityTrace   `json:"quality"`
}

type DebugPlanTrace struct {
	Terms            []string          `json:"terms,omitempty"`
	RetrievalQueries int               `json:"retrieval_queries"`
	GraphAnchors     int               `json:"graph_anchors"`
	SkipReasons      map[string]string `json:"skip_reasons,omitempty"`
}

type DebugRetrievalTrace struct {
	Candidates       int    `json:"candidates"`
	TopSource        string `json:"top_source,omitempty"`
	ContextAssembled bool   `json:"context_assembled"`
	GraphError       string `json:"graph_error,omitempty"`
}

type DebugQualityTrace struct {
	CRAGVerdict    string  `json:"crag_verdict"`
	CRAGConfidence float64 `json:"crag_confidence,omitempty"`
	CRAGMissing    string  `json:"crag_missing,omitempty"`
	Confidence     float64 `json:"confidence"`
	Threshold      float64 `json:"threshold"`
	Decision       string  `json:"decision"`
	DraftGenerated bool    `json:"draft_generated"`
	CriticRan      bool    `json:"critic_ran"`
}

type Planner interface {
	Plan(ctx context.Context, query Query) (QueryPlan, error)
}

type Retriever interface {
	Retrieve(ctx context.Context, query Query, meta Meta) ([]Candidate, error)
}

type GraphRetriever interface {
	RetrieveGraph(ctx context.Context, query Query, plan QueryPlan) ([]Candidate, error)
}

type RetrievalCost string

const (
	RetrievalCostCheap     RetrievalCost = "cheap"
	RetrievalCostExpensive RetrievalCost = "expensive"
)

type CostAwareRetriever interface {
	RetrievalCost() RetrievalCost
}

type Reranker interface {
	Rerank(ctx context.Context, query Query, meta Meta, candidates []Candidate) ([]Candidate, error)
}

type ThresholdForCompany func(ctx context.Context, companyID int64) float64

type Budget interface {
	Begin(ctx context.Context) context.Context
	Acquire(ctx context.Context, companyID int64, stage string) (bool, error)
}

type Config struct {
	Planner                Planner
	Retrievers             []Retriever
	GraphRetriever         GraphRetriever
	ContextAssembler       *ParentChildContextAssembler
	Reranker               Reranker
	Compressor             Compressor
	CRAG                   CRAG
	Generator              Generator
	Critic                 Critic
	ConfidenceThreshold    float64
	ConfidenceThresholdFor ThresholdForCompany
	Cache                  ports.Cache
	CacheTTL               time.Duration
	KnowledgeGap           ports.KnowledgeGapRecorder
	Budget                 Budget
	Workers                int
}
