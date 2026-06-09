package reply

import (
	"context"
	"fmt"
	"time"

	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

type AgentIDLookup func(ctx context.Context, companyID int64) (int64, bool)

type Workflow struct {
	pipeline   *rag.Pipeline
	actions    ports.AIActionRecorder
	agentIDFor AgentIDLookup
}

type Option func(*options)

type options struct {
	retrievers   []rag.Retriever
	graph        rag.GraphRetriever
	knowledgeGap ports.KnowledgeGapRecorder
	crag         rag.CRAG
	reranker     rag.Reranker
	generator    rag.Generator
	critic       rag.Critic
	thresholdFor rag.ThresholdForCompany
	actions      ports.AIActionRecorder
	agentIDFor   AgentIDLookup
}

func WithRetriever(retriever rag.Retriever) Option {
	return func(opts *options) {
		if retriever != nil {
			opts.retrievers = append(opts.retrievers, retriever)
		}
	}
}

func WithGraphRetriever(retriever rag.GraphRetriever) Option {
	return func(opts *options) {
		if retriever != nil {
			opts.graph = retriever
		}
	}
}

func WithKnowledgeGap(recorder ports.KnowledgeGapRecorder) Option {
	return func(opts *options) {
		opts.knowledgeGap = recorder
	}
}

func WithCRAG(crag rag.CRAG) Option {
	return func(opts *options) {
		if crag != nil {
			opts.crag = crag
		}
	}
}

func WithReranker(reranker rag.Reranker) Option {
	return func(opts *options) {
		if reranker != nil {
			opts.reranker = reranker
		}
	}
}

func WithGenerator(generator rag.Generator) Option {
	return func(opts *options) {
		if generator != nil {
			opts.generator = generator
		}
	}
}

func WithCritic(critic rag.Critic) Option {
	return func(opts *options) {
		if critic != nil {
			opts.critic = critic
		}
	}
}

func WithThresholdFor(fn rag.ThresholdForCompany) Option {
	return func(opts *options) {
		if fn != nil {
			opts.thresholdFor = fn
		}
	}
}

func WithActionRecorder(rec ports.AIActionRecorder, agentIDFor AgentIDLookup) Option {
	return func(opts *options) {
		if rec != nil && agentIDFor != nil {
			opts.actions = rec
			opts.agentIDFor = agentIDFor
		}
	}
}

func NewWorkflow(knowledge ports.KnowledgeRepository, opts ...Option) *Workflow {
	cfg := options{}
	for _, opt := range opts {
		opt(&cfg)
	}
	retrievers := make([]rag.Retriever, 0, 1+len(cfg.retrievers))
	retrievers = append(retrievers, rag.NewKnowledgeRetriever(knowledge))
	retrievers = append(retrievers, cfg.retrievers...)
	var contextAssembler rag.ContextAssembler
	if source, ok := knowledge.(rag.ParentChunkSource); ok {
		contextAssembler = rag.NewContextAssembler(source)
	}
	var reranker rag.Reranker = rag.LexicalReranker{}
	if cfg.reranker != nil {
		reranker = cfg.reranker
	}
	return &Workflow{
		pipeline: rag.NewPipeline(rag.Config{
			Extractor:              rag.DefaultExtractor{},
			Retrievers:             retrievers,
			GraphRetriever:         cfg.graph,
			ContextAssembler:       contextAssembler,
			Reranker:               reranker,
			Compressor:             rag.Compressor{MaxRunes: 2000},
			CRAG:                   cfg.crag,
			Generator:              cfg.generator,
			Critic:                 cfg.critic,
			ConfidenceThresholdFor: cfg.thresholdFor,
			KnowledgeGap:           cfg.knowledgeGap,
			Workers:                len(retrievers),
		}),
		actions:    cfg.actions,
		agentIDFor: cfg.agentIDFor,
	}
}

func (w *Workflow) Run(ctx context.Context, req dto.ReplyRequest) (dto.ReplyResponse, error) {
	if err := req.Validate(); err != nil {
		return dto.ReplyResponse{}, err
	}
	if err := ctx.Err(); err != nil {
		return dto.ReplyResponse{}, err
	}

	cid := ctxkey.MustCompanyID(ctx)

	start := time.Now()
	result, err := w.pipeline.Run(ctx, rag.Query{
		CompanyID: cid,
		Text:      req.Message,
		Limit:     3,
		Mode:      req.Mode,
		Debug:     req.Debug,
		Metadata:  req.Metadata,
	})
	if err != nil {
		return dto.ReplyResponse{}, fmt.Errorf("run rag pipeline: %w", err)
	}

	suggestion := result.Draft
	if suggestion == "" {
		suggestion = buildSuggestion(result.Meta.Intent, result.Candidates)
	}

	var actionID int64
	if w.actions != nil && w.agentIDFor != nil {
		actionID = w.recordAction(ctx, cid, req.Message, suggestion, result, time.Since(start))
	}

	return dto.ReplyResponse{
		Intent:         result.Meta.Intent,
		Draft:          suggestion,
		Suggestion:     suggestion,
		Sources:        toSources(result.Candidates),
		Confidence:     result.Confidence,
		Decision:       string(result.Decision),
		AIActionID:     actionID,
		StageTimingsMS: result.StageTimingsMS,
		DebugTrace:     result.DebugTrace,
	}, nil
}

func (w *Workflow) recordAction(ctx context.Context, companyID int64, query, draft string, result rag.Result, elapsed time.Duration) int64 {
	agentID, ok := w.agentIDFor(ctx, companyID)
	if !ok || agentID <= 0 {
		return 0
	}
	output := map[string]any{
		"draft":    draft,
		"intent":   result.Meta.Intent,
		"decision": string(result.Decision),
		"sources":  toActionSources(result.Candidates),
	}
	if result.Critique.Refusal != "" || len(result.Critique.Hallucinations) > 0 {
		output["critique"] = map[string]any{
			"supported":      result.Critique.Supported,
			"send":           result.Critique.Send,
			"refusal":        result.Critique.Refusal,
			"hallucinations": result.Critique.Hallucinations,
		}
	}
	input := map[string]any{"query": query}

	recCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	id, err := w.actions.Record(recCtx, ports.AIAction{
		CompanyID:  companyID,
		AgentID:    agentID,
		ActionType: "smart_reply",
		Input:      input,
		Output:     output,
		LatencyMs:  int(elapsed.Milliseconds()),
		Confidence: result.Confidence,
	})
	if err != nil {
		return 0
	}
	return id
}

func toActionSources(candidates []rag.Candidate) []ports.AIActionSource {
	out := make([]ports.AIActionSource, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, ports.AIActionSource{
			ID:        c.ID,
			Title:     c.Title,
			Score:     c.Score,
			Retriever: c.Source,
		})
	}
	return out
}

func buildSuggestion(intent string, candidates []rag.Candidate) string {
	if len(candidates) == 0 {
		return "Acknowledge the issue, confirm the affected service, and ask for the latest error details."
	}

	return fmt.Sprintf("Intent: %s. Suggested reply: %s", intent, candidates[0].Content)
}

func toSources(candidates []rag.Candidate) []dto.ReplySource {
	sources := make([]dto.ReplySource, 0, len(candidates))
	for _, candidate := range candidates {
		sources = append(sources, dto.ReplySource{
			ID:    candidate.ID,
			Title: candidate.Title,
			Score: candidate.Score,
		})
	}
	return sources
}
