package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/application/services/vectorindex"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	"github.com/my/app/internal/infra/qdrant"
	"github.com/my/app/internal/infra/tenant"
	mysqlkb "github.com/my/app/internal/repositories/kb/mysql"
	"github.com/my/app/internal/shared/config"
)

func main() {
	task := flag.String("task", "index-kb", "embedder task")
	company := flag.Int64("company", 0, "company id to scope indexing (0 = all)")
	collection := flag.String("collection", "", "qdrant collection")
	batchSize := flag.Int("batch-size", 64, "chunks per embedding batch")
	limit := flag.Int("limit", 0, "maximum chunks to index")
	provider := flag.String("provider", "auto", "embedding provider: auto, openai, openrouter, gemini, voyage, claude, ollama, hash")
	dim := flag.Int("dim", 0, "embedding dimensions")
	model := flag.String("model", "", "embedding model name")
	baseURL := flag.String("base-url", "", "OpenAI-compatible base URL")
	embedRPM := flag.Float64("embed-rpm", 0, "throttle embedding requests per minute (0 = no client-side throttle)")
	embedRetries := flag.Int("embed-retries", 0, "max retries on 429/5xx per batch (0 = provider default)")
	companies := flag.String("companies", "", "comma-separated company ids for batch tasks (overrides -company)")
	batchJob := flag.String("batch", "", "batch job name for embed-batch-collect (empty = all active)")
	maxPerJob := flag.Int("max-per-job", 0, "split batch submit into jobs of at most N requests (0 = single job; use to stay under free-tier per-job cap)")
	waveSize := flag.Int("wave-size", 1500, "embed-batch-run: chunks per wave (keep wave tokens under the batch enqueued-token pool)")
	dryRun := flag.Bool("dry-run", false, "count chunks without embedding")
	maxChunks := flag.Int("max-chunks", 0, "cap chunks indexed when -limit is empty (0 = use config default)")
	allowLarge := flag.Bool("allow-large-remote-run", false, "permit a remote-provider run above the large-run threshold")
	chunkRunes := flag.Int("chunk-runes", 0, "reembed: max runes per chunk (0 = chunker default)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if *collection == "" {
		*collection = cfg.Qdrant.CollectionPrefix
	}

	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	if db == nil {
		log.Fatal("mysql is disabled")
	}
	defer db.Close()
	qdb, closeRouter, err := buildTenantQuerier(cfg, db)
	if err != nil {
		log.Fatalf("build tenant router: %v", err)
	}
	defer closeRouter()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	switch *task {
	case "index-kb":
		resolvedProvider, modelName, embedder, err := embeddings.NewProvider(embeddingSettings(*provider, *model, *dim, *baseURL, *embedRPM, *embedRetries, cfg))
		if err != nil {
			log.Fatalf("build embedder: %v", err)
		}
		indexer := vectorindex.NewIndexer(
			mysqlkb.NewChunkSourceForModel(qdb, modelName),
			embedder,
			qdrant.NewStore(qdrant.Config{URL: cfg.Qdrant.URL, APIKey: cfg.Qdrant.APIKey}),
			mysqlkb.NewEmbeddingMarker(qdb),
		)
		if *company == 0 {
			log.Println("warning: -company=0 scans chunks across every company; pass -company=<id> to scope")
		}
		effectiveLimit := effectiveLimit(*limit, *maxChunks, cfg.Embedding.RefreshMaxChunks)
		if !*dryRun && !*allowLarge && isRemoteProvider(resolvedProvider) {
			if effectiveLimit == 0 || effectiveLimit > cfg.Embedding.LargeRunThreshold {
				log.Fatalf("refusing large remote embedding run (provider=%s limit=%d threshold=%d); pass -allow-large-remote-run, a smaller -max-chunks/-limit, or -dry-run first",
					resolvedProvider, effectiveLimit, cfg.Embedding.LargeRunThreshold)
			}
		}
		result, err := indexer.Run(ctx, vectorindex.Options{
			CompanyID:        *company,
			CollectionPrefix: *collection,
			BatchSize:        *batchSize,
			Limit:            effectiveLimit,
			Model:            modelName,
			DryRun:           *dryRun,
		})
		if err != nil {
			log.Fatalf("index kb: %v", err)
		}
		if *dryRun {
			tokens := result.EstimatedInputRunes / 4
			costUSD := float64(tokens) / 1_000_000 * 0.15
			fmt.Printf("kb index dry-run: chunks=%d batches=%d estimated_runes=%d estimated_tokens~=%d estimated_cost_usd~=%.4f (estimate only, gemini standard embedding)\n",
				result.Chunks, result.Batches, result.EstimatedInputRunes, tokens, costUSD)
		} else {
			fmt.Printf("kb index complete: chunks=%d points=%d batches=%d collection=%s model=%s provider=%s dim=%d\n", result.Chunks, result.Points, result.Batches, *collection, modelName, resolvedProvider, embedder.Dim())
		}
	case "reembed":
		if *company <= 0 {
			log.Fatal("reembed requires -company=<id> (re-embed is scoped to one tenant)")
		}
		resolvedProvider, modelName, embedder, err := embeddings.NewProvider(embeddingSettings(*provider, *model, *dim, *baseURL, *embedRPM, *embedRetries, cfg))
		if err != nil {
			log.Fatalf("build embedder: %v", err)
		}
		reembedder := vectorindex.NewReembedder(
			mysqlkb.NewReportSource(qdb),
			embedder,
			qdrant.NewStore(qdrant.Config{URL: cfg.Qdrant.URL, APIKey: cfg.Qdrant.APIKey}),
		)
		result, err := reembedder.Run(ctx, vectorindex.ReembedOptions{
			CompanyID:        *company,
			CollectionPrefix: *collection,
			ChunkRunes:       *chunkRunes,
			Limit:            *limit,
			Model:            modelName,
		})
		if err != nil {
			log.Fatalf("reembed: %v", err)
		}
		fmt.Printf("reembed complete: company=%d collection=%s dim=%d dimension=%s reports=%d chunks=%d points=%d provider=%s model=%s\n",
			*company, result.Collection, result.Dim, result.DimensionAction, result.Reports, result.Chunks, result.Points, resolvedProvider, modelName)
	case "embed-batch-submit":
		resolvedProvider, _, batchEmbedder, err := embeddings.NewBatchEmbedder(embeddingSettings(*provider, *model, *dim, *baseURL, *embedRPM, *embedRetries, cfg))
		if err != nil {
			log.Fatalf("build batch embedder: %v", err)
		}
		companyIDs := parseCompanies(*companies, *company)
		if len(companyIDs) == 0 {
			log.Fatal("embed-batch-submit requires -company=<id> or -companies=<id,id> (cross-tenant data is sharded; an explicit company set is required)")
		}
		source := mysqlkb.NewChunkSourceForModel(qdb, batchEmbedder.Model())
		reqs, err := collectBatchRequests(ctx, source, companyIDs, *batchSize, *limit)
		if err != nil {
			log.Fatalf("gather chunks: %v", err)
		}
		if len(reqs) == 0 {
			fmt.Printf("nothing to embed: no un-embedded chunks for companies %v (model=%s)\n", companyIDs, batchEmbedder.Model())
			return
		}
		repo := mysqlkb.NewEmbeddingBatchRepo(db)
		displayBase := fmt.Sprintf("kb-%s-%s-%d", resolvedProvider, batchEmbedder.Model(), time.Now().Unix())
		jobs := chunkRequests(reqs, *maxPerJob)
		created, submitted := 0, 0
		var orphaned []string
		for idx, chunk := range jobs {
			displayName := displayBase
			if len(jobs) > 1 {
				displayName = fmt.Sprintf("%s-p%d", displayBase, idx)
			}
			jobName, err := batchEmbedder.Submit(ctx, displayName, chunk)
			if err != nil {
				log.Printf("submit job %d/%d failed (%d created so far): %v", idx+1, len(jobs), created, err)
				break
			}
			if err := repo.Insert(ctx, mysqlkb.EmbeddingBatch{
				BatchName:    jobName,
				Provider:     resolvedProvider,
				Model:        batchEmbedder.Model(),
				Dimensions:   batchEmbedder.Dim(),
				DisplayName:  displayName,
				State:        "JOB_STATE_PENDING",
				RequestCount: len(chunk),
			}); err != nil {
				orphaned = append(orphaned, jobName)
				log.Printf("WARNING batch job %s submitted but not recorded: %v — collect or re-insert manually before it expires", jobName, err)
				continue
			}
			created++
			submitted += len(chunk)
			fmt.Printf("[%d/%d] %s (%d reqs)\n", idx+1, len(jobs), jobName, len(chunk))
			if idx+1 < len(jobs) {
				time.Sleep(3 * time.Second)
			}
		}
		fmt.Printf("submitted %d/%d jobs, %d/%d requests, companies=%v model=%s dim=%d\n", created, len(jobs), submitted, len(reqs), companyIDs, batchEmbedder.Model(), batchEmbedder.Dim())
		if len(orphaned) > 0 {
			fmt.Printf("WARNING %d submitted job(s) not recorded (recover manually): %v\n", len(orphaned), orphaned)
		}
		fmt.Println("collect with: -task=embed-batch-collect (drains all active jobs)")
	case "embed-batch-collect":
		_, _, batchEmbedder, err := embeddings.NewBatchEmbedder(embeddingSettings(*provider, *model, *dim, *baseURL, *embedRPM, *embedRetries, cfg))
		if err != nil {
			log.Fatalf("build batch embedder: %v", err)
		}
		repo := mysqlkb.NewEmbeddingBatchRepo(db)
		jobs, err := loadBatchJobs(ctx, repo, *batchJob)
		if err != nil {
			log.Fatalf("load batches: %v", err)
		}
		if len(jobs) == 0 {
			fmt.Println("no active batches to collect")
			return
		}
		ingestor := vectorindex.NewBatchIngestor(
			mysqlkb.NewChunkSourceForModel(qdb, batchEmbedder.Model()),
			qdrant.NewStore(qdrant.Config{URL: cfg.Qdrant.URL, APIKey: cfg.Qdrant.APIKey}),
			mysqlkb.NewEmbeddingMarker(qdb),
		)
		for _, job := range jobs {
			poll, err := batchEmbedder.Poll(ctx, job.BatchName)
			if err != nil {
				_ = repo.UpdateState(ctx, job.BatchName, pollState(poll.State), err.Error())
				log.Printf("batch %s failed: %v", job.BatchName, err)
				continue
			}
			if !poll.Done {
				_ = repo.UpdateState(ctx, job.BatchName, pollState(poll.State), "")
				fmt.Printf("batch %s: %s (not ready)\n", job.BatchName, poll.State)
				continue
			}
			res, err := ingestor.Ingest(ctx, vectorindex.Options{CollectionPrefix: *collection, Model: job.Model}, toKeyedVectors(poll.Results))
			if err != nil {
				_ = repo.UpdateState(ctx, job.BatchName, pollState(poll.State), err.Error())
				log.Printf("ingest batch %s failed; left active for retry: %v", job.BatchName, err)
				continue
			}
			if err := repo.MarkIngested(ctx, job.BatchName, res.Points); err != nil {
				log.Printf("batch %s ingested points=%d but state not recorded (will re-collect): %v", job.BatchName, res.Points, err)
				continue
			}
			if len(poll.Failed) > 0 {
				log.Printf("batch %s: %d key(s) failed at provider, left un-embedded for re-queue", job.BatchName, len(poll.Failed))
			}
			fmt.Printf("batch %s ingested: points=%d companies=%d collection=%s model=%s\n", job.BatchName, res.Points, res.Batches, *collection, job.Model)
		}
	case "embed-batch-run":
		resolvedProvider, _, batchEmbedder, err := embeddings.NewBatchEmbedder(embeddingSettings(*provider, *model, *dim, *baseURL, *embedRPM, *embedRetries, cfg))
		if err != nil {
			log.Fatalf("build batch embedder: %v", err)
		}
		companyIDs := parseCompanies(*companies, *company)
		if len(companyIDs) == 0 {
			log.Fatal("embed-batch-run requires -company=<id> or -companies=<id,id>")
		}
		repo := mysqlkb.NewEmbeddingBatchRepo(db)
		source := mysqlkb.NewChunkSourceForModel(qdb, batchEmbedder.Model())
		ingestor := vectorindex.NewBatchIngestor(
			source,
			qdrant.NewStore(qdrant.Config{URL: cfg.Qdrant.URL, APIKey: cfg.Qdrant.APIKey}),
			mysqlkb.NewEmbeddingMarker(qdb),
		)

		active, err := repo.ListActive(ctx)
		if err != nil {
			log.Fatalf("list active batches: %v", err)
		}
		inFlight := 0
		for _, job := range active {
			poll, err := batchEmbedder.Poll(ctx, job.BatchName)
			if err != nil {
				_ = repo.UpdateState(ctx, job.BatchName, pollState(poll.State), err.Error())
				log.Printf("batch %s %s; its chunks return to the queue", job.BatchName, pollState(poll.State))
				continue
			}
			if !poll.Done {
				_ = repo.UpdateState(ctx, job.BatchName, pollState(poll.State), "")
				inFlight++
				continue
			}
			res, err := ingestor.Ingest(ctx, vectorindex.Options{CollectionPrefix: *collection, Model: job.Model}, toKeyedVectors(poll.Results))
			if err != nil {
				_ = repo.UpdateState(ctx, job.BatchName, pollState(poll.State), err.Error())
				log.Printf("ingest batch %s failed; left active for retry: %v", job.BatchName, err)
				continue
			}
			if err := repo.MarkIngested(ctx, job.BatchName, res.Points); err != nil {
				log.Printf("batch %s ingested points=%d but state not recorded (will re-collect): %v", job.BatchName, res.Points, err)
				continue
			}
			if len(poll.Failed) > 0 {
				log.Printf("batch %s: %d key(s) failed at provider, left un-embedded for re-queue", job.BatchName, len(poll.Failed))
			}
			fmt.Printf("ingested %s: points=%d\n", job.BatchName, res.Points)
		}

		if inFlight > 0 {
			fmt.Printf("%d job(s) still in flight; pool busy — re-run after they finish\n", inFlight)
			return
		}

		reqs, err := collectBatchRequests(ctx, source, companyIDs, *batchSize, *waveSize)
		if err != nil {
			log.Fatalf("gather chunks: %v", err)
		}
		if len(reqs) == 0 {
			fmt.Printf("all chunks embedded for companies %v (model=%s) — nothing to do\n", companyIDs, batchEmbedder.Model())
			return
		}
		displayName := fmt.Sprintf("kb-%s-%s-%d", resolvedProvider, batchEmbedder.Model(), time.Now().Unix())
		jobName, err := batchEmbedder.Submit(ctx, displayName, reqs)
		if err != nil {
			fmt.Printf("wave not submitted (%d chunks pending): %v\nenqueue pool likely busy — re-run later\n", len(reqs), err)
			return
		}
		if err := repo.Insert(ctx, mysqlkb.EmbeddingBatch{
			BatchName:    jobName,
			Provider:     resolvedProvider,
			Model:        batchEmbedder.Model(),
			Dimensions:   batchEmbedder.Dim(),
			DisplayName:  displayName,
			State:        "JOB_STATE_PENDING",
			RequestCount: len(reqs),
		}); err != nil {
			log.Printf("WARNING wave %s submitted but not recorded: %v — collect or re-insert manually before it expires", jobName, err)
			return
		}
		fmt.Printf("wave submitted %s: %d chunks (companies=%v model=%s). Re-run after it completes to continue.\n", jobName, len(reqs), companyIDs, batchEmbedder.Model())
	default:
		log.Fatalf("unknown task %q", *task)
	}
}

func toKeyedVectors(results []embeddings.BatchEmbedResult) []vectorindex.KeyedVector {
	kvs := make([]vectorindex.KeyedVector, 0, len(results))
	for _, r := range results {
		kvs = append(kvs, vectorindex.KeyedVector{Key: r.Key, Vector: r.Values})
	}
	return kvs
}

func parseCompanies(csv string, single int64) []int64 {
	if strings.TrimSpace(csv) != "" {
		var ids []int64
		for part := range strings.SplitSeq(csv, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err == nil && id > 0 {
				ids = append(ids, id)
			}
		}
		return ids
	}
	if single > 0 {
		return []int64{single}
	}
	return nil
}

func collectBatchRequests(ctx context.Context, source *mysqlkb.ChunkSource, companyIDs []int64, batchSize, limit int) ([]embeddings.BatchEmbedRequest, error) {
	if batchSize <= 0 {
		batchSize = 500
	}
	var reqs []embeddings.BatchEmbedRequest
	for _, companyID := range companyIDs {
		var afterID int64
		for {
			if limit > 0 && len(reqs) >= limit {
				return reqs[:limit], nil
			}
			chunks, err := source.Next(ctx, companyID, afterID, batchSize)
			if err != nil {
				return nil, err
			}
			if len(chunks) == 0 {
				break
			}
			for _, chunk := range chunks {
				reqs = append(reqs, embeddings.BatchEmbedRequest{
					Key:  vectorindex.FormatBatchKey(companyID, chunk.ID),
					Text: chunk.Content,
				})
			}
			afterID = chunks[len(chunks)-1].ID
		}
	}
	return reqs, nil
}

func chunkRequests(reqs []embeddings.BatchEmbedRequest, maxPerJob int) [][]embeddings.BatchEmbedRequest {
	if maxPerJob <= 0 || len(reqs) <= maxPerJob {
		return [][]embeddings.BatchEmbedRequest{reqs}
	}
	var jobs [][]embeddings.BatchEmbedRequest
	for start := 0; start < len(reqs); start += maxPerJob {
		end := min(start+maxPerJob, len(reqs))
		jobs = append(jobs, reqs[start:end])
	}
	return jobs
}

func loadBatchJobs(ctx context.Context, repo *mysqlkb.EmbeddingBatchRepo, name string) ([]mysqlkb.EmbeddingBatch, error) {
	if name != "" {
		job, err := repo.GetByName(ctx, name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("no batch named %q", name)
			}
			return nil, err
		}
		return []mysqlkb.EmbeddingBatch{job}, nil
	}
	return repo.ListActive(ctx)
}

func buildTenantQuerier(cfg config.Config, db *sql.DB) (tenant.Querier, func(), error) {
	pool, err := tenant.NewPool(db, func(name string) (*sql.DB, error) {
		return infra_mysql.OpenForDB(cfg.MySQL, name)
	})
	if err != nil {
		return nil, nil, err
	}
	return pool.Router(), pool.Close, nil
}

func embeddingSettings(provider string, model string, dim int, baseURL string, rpm float64, retries int, cfg config.Config) embeddings.ProviderSettings {
	if provider == "auto" && cfg.LLM.EmbeddingProvider != "" {
		provider = cfg.LLM.EmbeddingProvider
	}
	if model == "" {
		model = cfg.LLM.EmbeddingModel
	}
	if dim <= 0 {
		dim = cfg.LLM.EmbeddingDim
	}
	return embeddings.ProviderSettings{
		Provider:       provider,
		Model:          model,
		BaseURL:        baseURL,
		Dimensions:     dim,
		MaxRetries:     retries,
		RequestsPerMin: rpm,
		OpenAIKey:      firstNonEmpty(os.Getenv("OPENAI_API_KEY"), cfg.APIKeys.OpenAI, cfg.LLM.OpenAIKey),
		GeminiKey:      firstNonEmpty(os.Getenv("GEMINI_API_KEY"), cfg.APIKeys.Gemini, cfg.LLM.GeminiKey),
		VoyageKey:      firstNonEmpty(os.Getenv("VOYAGE_API_KEY"), cfg.APIKeys.Voyage),
		OpenRouterKey:  firstNonEmpty(os.Getenv("OPENROUTER_API_KEY"), cfg.APIKeys.OpenRouter),
		LocalURL:       cfg.LLM.LocalURL,
	}
}

func effectiveLimit(limit, maxChunks, configMax int) int {
	if limit > 0 {
		return limit
	}
	if maxChunks > 0 {
		return maxChunks
	}
	return configMax
}

func pollState(state string) string {
	if strings.TrimSpace(state) == "" {
		return "POLL_ERROR"
	}
	return state
}

func isRemoteProvider(provider string) bool {
	switch provider {
	case "hash", "ollama", "local":
		return false
	default:
		return true
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
