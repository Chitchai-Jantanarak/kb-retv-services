package main

import "flag"

func parseFlags() commandOptions {
	task := flag.String("task", commandIndexKB, "embedder task")
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

	return commandOptions{
		task:                *task,
		company:             *company,
		collection:          *collection,
		batchSize:           *batchSize,
		limit:               *limit,
		provider:            *provider,
		dim:                 *dim,
		model:               *model,
		baseURL:             *baseURL,
		embedRPM:            *embedRPM,
		embedRetries:        *embedRetries,
		companies:           *companies,
		batchJob:            *batchJob,
		maxPerJob:           *maxPerJob,
		waveSize:            *waveSize,
		dryRun:              *dryRun,
		maxChunks:           *maxChunks,
		allowLargeRemoteRun: *allowLarge,
		chunkRunes:          *chunkRunes,
	}
}
