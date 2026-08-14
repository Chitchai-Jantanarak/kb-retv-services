package chat

import (
	"context"
	"time"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/intent"
	"github.com/my/app/internal/application/skeleton"
	"github.com/my/app/internal/domain/ports"
	chatsession "github.com/my/app/internal/repositories/chatsession/mysql"
)

type SessionStore interface {
	EnsureSession(ctx context.Context, companyID, userID, sessionID int64) (int64, error)
	LastCites(ctx context.Context, companyID, userID, sessionID int64) (string, []string, error)
	AppendTurn(ctx context.Context, turn chatsession.Turn) error
}

type toolRunner interface {
	Handle(ctx context.Context, actor skeleton.Actor, msg string) (skeleton.Response, error)
}

type ProfileSource interface {
	Build(ctx context.Context, companyID int64) (string, error)
}

type CaseSource interface {
	SearchCases(ctx context.Context, coverage []int64, query, product, status string, limit int) ([]dto.ChatCaseResult, error)
}

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
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

func WithTurnRecorder(rec TurnRecorder) Option {
	return func(w *Workflow) { w.turns = rec }
}

func WithSessions(store SessionStore) Option {
	return func(w *Workflow) { w.sessions = store }
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

func WithKnowledgeReranker(emb Embedder) Option {
	return func(w *Workflow) {
		if emb != nil {
			w.knowledgeReranker = emb
		}
	}
}

func WithReplyPool(provider ports.LLMProvider) Option {
	return func(w *Workflow) {
		if provider == nil {
			return
		}
		w.pool = newReplyPool(provider)
		go w.pool.warm(context.Background())
	}
}

func WithOffTopicMargin(margin float64) Option {
	return func(w *Workflow) {
		if margin > 0 {
			w.offTopicMargin = margin
		}
	}
}
