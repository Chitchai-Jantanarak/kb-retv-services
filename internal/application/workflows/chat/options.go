package chat

import (
	"context"
	"time"

	"github.com/my/app/internal/ai/rag"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/intent"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/domain/ports"
)

type toolRunner interface {
	Handle(ctx context.Context, actor skeleton.Actor, msg string) (skeleton.Response, error)
}

type FTSSource interface {
	SearchChunks(ctx context.Context, coverage []int64, query string, limit int) ([]rag.FTSChunk, error)
}

type ProfileSource interface {
	Build(ctx context.Context, companyID int64) (string, error)
}

type CaseSource interface {
	SearchCases(ctx context.Context, coverage []int64, query, product, status string, limit int) ([]dto.ChatCaseResult, error)
}

type Option func(*Workflow)

func WithCache(cache ports.Cache, ttl time.Duration) Option {
	return func(w *Workflow) {
		if cache != nil && ttl > 0 {
			w.cache = cache
			w.cacheTTL = ttl
		}
	}
}

func WithRouter(router *intent.Router) Option {
	return func(w *Workflow) { w.router = router }
}

func WithOrchestrator(orch toolRunner) Option {
	return func(w *Workflow) { w.orch = orch }
}

func WithProfile(profile ProfileSource) Option {
	return func(w *Workflow) {
		if profile != nil {
			w.profile = profile
		}
	}
}

func WithCaseSearch(cases CaseSource) Option {
	return func(w *Workflow) {
		if cases != nil {
			w.cases = cases
		}
	}
}

func WithTranscription(fetcher ports.AttachmentFetcher, transcriber ports.Transcriber) Option {
	return func(w *Workflow) {
		if fetcher != nil && transcriber != nil {
			w.fetcher = fetcher
			w.transcriber = transcriber
		}
	}
}
