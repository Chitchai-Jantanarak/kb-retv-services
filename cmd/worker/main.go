package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/application/services/kbbootstrap"
	"github.com/my/app/internal/application/services/reviewoutbox"
	"github.com/my/app/internal/application/services/tickets"
	"github.com/my/app/internal/application/services/vectorindex"
	"github.com/my/app/internal/application/workflows/classify"
	"github.com/my/app/internal/application/workflows/graphsync"
	"github.com/my/app/internal/application/workflows/ingest"
	"github.com/my/app/internal/application/workflows/promote"
	"github.com/my/app/internal/domain/ports"
	infraasynq "github.com/my/app/internal/infra/asynq"
	"github.com/my/app/internal/infra/memgraph"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	"github.com/my/app/internal/infra/qdrant"
	"github.com/my/app/internal/infra/tenant"
	classifymysql "github.com/my/app/internal/repositories/classify/mysql"
	"github.com/my/app/internal/repositories/graph"
	mysqlkb "github.com/my/app/internal/repositories/kb/mysql"
	reviewmysql "github.com/my/app/internal/repositories/review/mysql"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

const (
	TaskReplyLine        = "reply:line"
	TaskReplyEmail       = "reply:email"
	TaskIngestReport     = "ingest:report"
	TaskExtractBatch     = "extract:batch"
	TaskClusterWeekly    = "cluster:weekly"
	TaskPromoteKB        = "promote:kb"
	TaskPromoteSymptom   = "promote:symptom"
	TaskPromoteSubject   = "promote:subject"
	TaskOutboxFlush      = "outbox:flush"
	TaskEmbeddingRefresh = "embedding:refresh"
)

type taskHandler func(ctx context.Context, job ports.Job) error

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	redisURL := cfg.Redis.URL
	if redisURL == "" {
		redisURL = os.Getenv("REDIS_URL")
	}
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		log.Fatalf("parse redis url: %v", err)
	}

	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency: 10,
		Queues: map[string]int{
			"critical": 6,
			"default":  3,
			"low":      1,
		},
	})

	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskReplyLine, makeHandler(stubHandler("reply:line")))
	mux.HandleFunc(TaskReplyEmail, makeHandler(stubHandler("reply:email")))
	mux.HandleFunc(TaskIngestReport, makeHandler(buildIngestHandler(cfg)))
	mux.HandleFunc(TaskExtractBatch, makeHandler(buildExtractHandler(cfg)))
	mux.HandleFunc(TaskClusterWeekly, makeHandler(stubHandler("cluster:weekly")))
	promoteFn := buildPromoteHandler(cfg)
	mux.HandleFunc(TaskPromoteKB, makeHandler(promoteFn))
	mux.HandleFunc(TaskPromoteSymptom, makeHandler(promoteFn))
	mux.HandleFunc(TaskPromoteSubject, makeHandler(promoteFn))
	mux.HandleFunc(TaskOutboxFlush, makeHandler(buildOutboxHandler(cfg)))
	mux.HandleFunc(TaskEmbeddingRefresh, makeHandler(buildRefreshHandler(cfg)))
	mux.HandleFunc(tickets.TaskCreate, makeHandler(buildTicketCreateHandler(cfg)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("worker starting (redis=%s)", redisURL)
		errCh <- srv.Run(mux)
	}()

	select {
	case <-ctx.Done():
		log.Println("worker shutting down")
		srv.Shutdown()
	case err := <-errCh:
		log.Fatalf("worker error: %v", err)
	}
}

func makeHandler(fn taskHandler) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		if err := ctx.Err(); err != nil {
			log.Printf("task %s skipped: context already done: %v", t.Type(), err)
			return err
		}

		job, err := infraasynq.ExtractJob(t)
		if err != nil {
			log.Printf("task %s decode error: %v", t.Type(), err)
			return err
		}

		start := time.Now()
		log.Printf("task start type=%s company=%d trace=%s", job.Type, job.CompanyID, job.TraceID)

		handlerErr := fn(ctx, job)

		elapsed := time.Since(start)
		if handlerErr != nil {
			log.Printf("task error type=%s company=%d elapsed=%s err=%v", job.Type, job.CompanyID, elapsed, handlerErr)
		} else {
			log.Printf("task done type=%s company=%d elapsed=%s", job.Type, job.CompanyID, elapsed)
		}
		return handlerErr
	}
}

func stubHandler(name string) taskHandler {
	return func(_ context.Context, job ports.Job) error {
		log.Printf("TODO handler type=%s company=%d", name, job.CompanyID)
		return nil
	}
}

func unavailable(name, reason string) taskHandler {
	err := fmt.Errorf("%s unavailable: %s", name, reason)
	return func(_ context.Context, _ ports.Job) error { return err }
}

func buildIngestHandler(cfg config.Config) taskHandler {
	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil {
		log.Printf("ingest disabled: open mysql: %v", err)
		return unavailable("ingest", "mysql open failed")
	}
	if db == nil {
		log.Println("ingest disabled: mysql is not configured")
		return unavailable("ingest", "mysql not configured")
	}

	provider, modelName, embedder, err := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg))
	if err != nil {
		log.Printf("ingest disabled: build embedder: %v", err)
		return unavailable("ingest", "embedder unavailable")
	}

	collectionPrefix := cfg.Qdrant.CollectionPrefix
	if collectionPrefix == "" {
		collectionPrefix = "kb_chunks"
	}

	wf, err := ingest.New(ingest.Config{
		Bootstrapper: kbbootstrap.NewBootstrapper(
			mysqlkb.NewReportSource(db),
			mysqlkb.NewArticleStore(db),
		),
		Indexer: vectorindex.NewIndexer(
			mysqlkb.NewChunkSourceForModel(db, modelName),
			embedder,
			qdrant.NewStore(qdrant.Config{URL: cfg.Qdrant.URL, APIKey: cfg.Qdrant.APIKey}),
			mysqlkb.NewEmbeddingMarker(db),
		),
		CollectionPrefix: collectionPrefix,
		Model:            modelName,
		ChunkRunes:       kbbootstrap.DefaultChunkRunes,
	})
	if err != nil {
		log.Printf("ingest disabled: build workflow: %v", err)
		return unavailable("ingest", "workflow build failed")
	}

	log.Printf("ingest handler configured: provider=%s model=%s collection_prefix=%s",
		provider, modelName, collectionPrefix)

	graphWf := buildGraphsyncWorkflow(cfg, db)
	if graphWf != nil {
		log.Println("graphsync configured: ingest:report will sync to memgraph after successful index")
	}

	return func(ctx context.Context, job ports.Job) error {
		p, err := decodeIngestPayload(job.Payload)
		if err != nil {
			return err
		}
		since := time.Now().Add(-5 * time.Second)
		result, err := wf.Run(ctx, job.CompanyID, ingest.Options{
			Limit:     p.Limit,
			BatchSize: p.BatchSize,
			DryRun:    p.DryRun,
		})
		if err != nil {
			return err
		}
		log.Printf("ingest result company=%d dry_run=%v reports=%d articles=%d skipped=%d chunks=%d indexed_points=%d batches=%d",
			job.CompanyID, p.DryRun, result.Reports, result.Articles, result.ArticleSkipped,
			result.IndexedChunks, result.IndexedPoints, result.IndexBatches)

		if !p.DryRun && graphWf != nil && result.Articles > 0 {
			runGraphsync(ctx, graphWf, job.CompanyID, since, result.Articles)
		}
		return nil
	}
}

func buildGraphsyncWorkflow(cfg config.Config, db *sql.DB) *graphsync.Workflow {
	if !cfg.Memgraph.Enabled || db == nil {
		return nil
	}
	store, err := memgraph.NewStore(memgraph.Config{
		URI:      cfg.Memgraph.URI,
		Username: cfg.Memgraph.Username,
		Password: cfg.Memgraph.Password,
	})
	if err != nil {
		log.Printf("graphsync disabled: memgraph open: %v", err)
		return nil
	}
	wf, err := graphsync.New(graphsync.Config{
		Source: mysqlkb.NewArticleSource(db),
		Graph:  graph.NewKBGraph(store),
	})
	if err != nil {
		log.Printf("graphsync disabled: build workflow: %v", err)
		return nil
	}
	return wf
}

func runGraphsync(ctx context.Context, wf *graphsync.Workflow, companyID int64, since time.Time, articleCount int) {
	res, err := wf.Run(ctx, graphsync.Options{
		CompanyID: companyID,
		Since:     since,
		Limit:     articleCount + 16,
	})
	if err != nil {
		log.Printf("graphsync error company=%d err=%v", companyID, err)
		return
	}
	log.Printf("graphsync result company=%d articles=%d edges=%d skipped=%d",
		companyID, res.Articles, res.Edges, res.Skipped)
}

func buildRefreshHandler(cfg config.Config) taskHandler {
	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil || db == nil {
		log.Printf("embedding:refresh disabled: mysql unavailable")
		return unavailable("embedding:refresh", "mysql not configured")
	}
	qdb, err := buildTenantQuerier(cfg, db)
	if err != nil {
		log.Printf("embedding:refresh disabled: tenant router unavailable: %v", err)
		return unavailable("embedding:refresh", "tenant router not configured")
	}
	provider, modelName, embedder, err := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg))
	if err != nil {
		log.Printf("embedding:refresh disabled: build embedder: %v", err)
		return unavailable("embedding:refresh", "embedder unavailable")
	}
	collectionPrefix := cfg.Qdrant.CollectionPrefix
	if collectionPrefix == "" {
		collectionPrefix = "kb_chunks"
	}
	indexer := vectorindex.NewIndexer(
		mysqlkb.NewChunkSourceForModel(qdb, modelName),
		embedder,
		qdrant.NewStore(qdrant.Config{URL: cfg.Qdrant.URL, APIKey: cfg.Qdrant.APIKey}),
		mysqlkb.NewEmbeddingMarker(qdb),
	)
	log.Printf("embedding:refresh handler configured: provider=%s model=%s", provider, modelName)

	return func(ctx context.Context, job ports.Job) error {
		res, err := indexer.Run(ctx, vectorindex.Options{
			CompanyID:        job.CompanyID,
			CollectionPrefix: collectionPrefix,
			Model:            modelName,
		})
		if err != nil {
			return fmt.Errorf("embedding:refresh: %w", err)
		}
		log.Printf("embedding:refresh result company=%d chunks=%d points=%d batches=%d",
			job.CompanyID, res.Chunks, res.Points, res.Batches)
		return nil
	}
}

func buildTenantQuerier(cfg config.Config, db *sql.DB) (tenant.Querier, error) {
	pool, err := tenant.NewPool(db, func(name string) (*sql.DB, error) {
		return infra_mysql.OpenForDB(cfg.MySQL, name)
	})
	if err != nil {
		return nil, err
	}
	return pool.Router(), nil
}

func buildTicketCreateHandler(cfg config.Config) taskHandler {
	deliverer, err := tickets.NewDeliverer(tickets.DeliveryConfig{
		BaseURL: cfg.Laravel.BaseURL,
		Path:    cfg.Laravel.TicketPath,
		Secret:  cfg.Laravel.WebhookSecret,
		Timeout: time.Duration(cfg.Laravel.Timeout) * time.Second,
	})
	if err != nil {
		log.Printf("ticket:create disabled: %v", err)
		return unavailable("ticket:create", err.Error())
	}
	log.Printf("ticket:create handler configured: base=%s path=%s", cfg.Laravel.BaseURL, cfg.Laravel.TicketPath)

	return func(ctx context.Context, job ports.Job) error {
		var p tickets.Payload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return fmt.Errorf("ticket:create: decode payload: %w", err)
		}
		if p.CompanyID <= 0 {
			p.CompanyID = job.CompanyID
		}
		if p.CompanyID != job.CompanyID {
			return fmt.Errorf("ticket:create: payload company %d != job company %d", p.CompanyID, job.CompanyID)
		}

		body, err := buildTicketBody(p)
		if err != nil {
			return err
		}
		return deliverer.Deliver(ctx, body)
	}
}

func buildOutboxHandler(cfg config.Config) taskHandler {
	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil || db == nil {
		log.Printf("outbox:flush disabled: mysql unavailable")
		return unavailable("outbox:flush", "mysql not configured")
	}
	deliverer, err := reviewoutbox.NewDeliverer(reviewoutbox.DeliveryConfig{
		BaseURL: cfg.Laravel.BaseURL,
		Secret:  cfg.Laravel.WebhookSecret,
		Timeout: time.Duration(cfg.Laravel.Timeout) * time.Second,
	})
	if err != nil {
		log.Printf("outbox:flush disabled: %v", err)
		return unavailable("outbox:flush", err.Error())
	}
	flusher := reviewoutbox.NewFlusher(reviewmysql.NewOutboxWriter(db), deliverer)
	log.Println("outbox:flush handler configured")

	return func(ctx context.Context, job ports.Job) error {
		p, err := decodeIngestPayload(job.Payload)
		if err != nil {
			return fmt.Errorf("outbox:flush: %w", err)
		}
		res, err := flusher.Run(ctx, reviewoutbox.Options{
			CompanyID: job.CompanyID,
			Limit:     p.Limit,
		})
		if err != nil {
			return err
		}
		log.Printf("outbox:flush result company=%d scanned=%d sent=%d failed=%d",
			job.CompanyID, res.Scanned, res.Sent, res.Failed)
		return nil
	}
}

func buildTicketBody(p tickets.Payload) ([]byte, error) {
	envelope := map[string]any{
		"company_id":      p.CompanyID,
		"channel":         p.Channel,
		"conversation_id": p.ConversationID,
		"message_id":      p.MessageID,
		"subject":         p.Subject,
		"body":            p.Body,
		"sender_external": p.SenderExternal,
		"sender_name":     p.SenderName,
		"idempotency_key": p.IdempotencyKey(),
	}
	b, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("ticket:create: marshal envelope: %w", err)
	}
	return b, nil
}

func buildPromoteHandler(cfg config.Config) taskHandler {
	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil || db == nil {
		log.Printf("promote:* disabled: mysql unavailable")
		return unavailable("promote", "mysql not configured")
	}
	wf, err := promote.New(reviewmysql.NewReviewRepository(db))
	if err != nil {
		log.Printf("promote:* disabled: build workflow: %v", err)
		return unavailable("promote", "workflow build failed")
	}
	log.Println("promote:* handler configured")

	return func(ctx context.Context, job ports.Job) error {
		var p struct {
			ReviewItemID int64 `json:"review_item_id"`
		}
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return fmt.Errorf("promote: decode payload: %w", err)
		}
		res, err := wf.Run(ctx, p.ReviewItemID)
		if err != nil {
			return err
		}
		log.Printf("promote result task=%s company=%d review_item=%d kind=%s ref=%s",
			job.Type, job.CompanyID, res.ReviewItemID, res.Kind, res.PromotionRef)
		return nil
	}
}

func buildExtractHandler(cfg config.Config) taskHandler {
	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil || db == nil {
		log.Printf("extract:batch disabled: mysql unavailable")
		return unavailable("extract:batch", "mysql not configured")
	}
	resolver, err := llmboot.Resolver(cfg, db)
	if err != nil {
		log.Printf("extract:batch disabled: %v", err)
		return unavailable("extract:batch", "resolver failed")
	}
	registry, err := prompts.NewRegistry()
	if err != nil {
		log.Printf("extract:batch disabled: load prompts: %v", err)
		return unavailable("extract:batch", "prompts failed")
	}
	repo := classifymysql.New(db)
	wf, err := classify.New(classify.Config{
		Registry: registry,
		Source:   repo,
		Lookup:   repo,
		Sink:     repo,
		Resolve:  resolver.ResolveFor,
		Review:   reviewmysql.NewOutboxWriter(db),
		Model:    cfg.LLM.DefaultModel,
	})
	if err != nil {
		log.Printf("extract:batch disabled: build workflow: %v", err)
		return unavailable("extract:batch", "workflow build failed")
	}
	log.Println("extract:batch handler configured")

	return func(ctx context.Context, job ports.Job) error {
		p, err := decodeIngestPayload(job.Payload)
		if err != nil {
			return fmt.Errorf("extract:batch: %w", err)
		}
		res, err := wf.Run(ctx, classify.Options{
			CompanyID: job.CompanyID,
			Limit:     p.Limit,
		})
		if err != nil {
			return err
		}
		log.Printf("extract:batch result company=%d examined=%d classified=%d failed=%d",
			job.CompanyID, res.Examined, res.Classified, res.Failed)
		return nil
	}
}

func decodeIngestPayload(raw []byte) (dto.IngestReportPayload, error) {
	var p dto.IngestReportPayload
	if len(raw) == 0 {
		return p, nil
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("decode payload: %w", err)
	}
	return p, nil
}
