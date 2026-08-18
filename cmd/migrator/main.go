package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/my/app/internal/ai/embeddings"
	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/application/services/kbbootstrap"
	"github.com/my/app/internal/application/workflows/classify"
	"github.com/my/app/internal/application/workflows/graphsync"
	"github.com/my/app/internal/application/workflows/symptommerge"
	"github.com/my/app/internal/infra/memgraph"
	infra_mysql "github.com/my/app/internal/infra/mysql"
	classifymysql "github.com/my/app/internal/repositories/classify/mysql"
	"github.com/my/app/internal/repositories/graph"
	mysqlkb "github.com/my/app/internal/repositories/kb/mysql"
	reviewmysql "github.com/my/app/internal/repositories/review/mysql"
	symptommysql "github.com/my/app/internal/repositories/symptom/mysql"
	"github.com/my/app/internal/shared/config"
	"github.com/my/app/internal/shared/llmboot"
)

func main() {
	task := flag.String("task", "", "migration task")
	company := flag.Int64("company", 0, "company id to scope the task (0 = all where supported)")
	limit := flag.Int("limit", 0, "maximum records to process")
	chunkRunes := flag.Int("chunk-runes", kbbootstrap.DefaultChunkRunes, "chunk size in runes")
	sinceText := flag.String("since", "", "graph-sync lower bound, RFC3339 or YYYY-MM-DD")
	timeout := flag.Duration("timeout", 10*time.Minute, "overall task deadline, e.g. 45m or 2h")
	threshold := flag.Float64("threshold", 0.85, "symptom-merge cosine threshold")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := infra_mysql.Open(cfg.MySQL)
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	if db == nil {
		log.Fatal("mysql is disabled")
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	switch *task {
	case "kb-bootstrap":
		source := mysqlkb.NewReportSource(db)
		store := mysqlkb.NewArticleStore(db)
		bootstrapper := kbbootstrap.NewBootstrapper(source, store)
		if *company == 0 {
			log.Println("warning: -company=0 reads reports from every company; pass -company=<id> to scope")
		}
		result, err := bootstrapper.Run(ctx, kbbootstrap.Options{
			CompanyID:  *company,
			Limit:      *limit,
			ChunkRunes: *chunkRunes,
		})
		if err != nil {
			log.Fatalf("kb bootstrap: %v", err)
		}
		fmt.Printf("kb bootstrap complete: reports=%d articles=%d chunks=%d skipped=%d\n", result.Reports, result.Articles, result.Chunks, result.Skipped)
	case "symptom-merge":
		if *company <= 0 {
			log.Fatal("symptom merge: -company must be positive")
		}
		_, _, embedder, err := embeddings.NewProvider(llmboot.EmbeddingSettings(cfg))
		if err != nil {
			log.Fatalf("symptom merge: embedder: %v", err)
		}
		workflow, err := symptommerge.New(symptommerge.Config{
			Store:     symptommysql.New(db),
			Embedder:  embedder,
			Threshold: *threshold,
		})
		if err != nil {
			log.Fatalf("symptom merge: build workflow: %v", err)
		}
		result, err := workflow.Run(ctx, *company)
		if err != nil {
			log.Fatalf("symptom merge: %v", err)
		}
		fmt.Printf("symptom merge complete: company=%d examined=%d canonical=%d merged=%d\n", *company, result.Examined, result.Canonical, result.Merged)
	case "graph-sync":
		if *company <= 0 {
			log.Fatal("graph sync: -company must be positive")
		}
		since, err := parseSince(*sinceText)
		if err != nil {
			log.Fatalf("graph sync: %v", err)
		}
		uri := cfg.Memgraph.URI
		if uri == "" {
			log.Fatal("graph sync: MEMGRAPH_URI or memgraph.uri is required")
		}
		store, err := memgraph.NewStore(memgraph.Config{
			URI:      uri,
			Username: cfg.Memgraph.Username,
			Password: cfg.Memgraph.Password,
		})
		if err != nil {
			log.Fatalf("open memgraph: %v", err)
		}
		defer func() { _ = store.Close(ctx) }()
		if err := store.VerifyConnectivity(ctx); err != nil {
			log.Fatalf("verify memgraph: %v", err)
		}
		workflow, err := graphsync.New(graphsync.Config{
			Source: mysqlkb.NewArticleSource(db),
			Graph:  graph.NewKBGraph(store),
		})
		if err != nil {
			log.Fatalf("build graph sync: %v", err)
		}
		result, err := workflow.Run(ctx, graphsync.Options{
			CompanyID: *company,
			Since:     since,
			Limit:     *limit,
		})
		if err != nil {
			log.Fatalf("graph sync: %v", err)
		}
		fmt.Printf("graph sync complete: company=%d articles=%d edges=%d skipped=%d\n", *company, result.Articles, result.Edges, result.Skipped)
	case "classify":
		if *company <= 0 {
			log.Fatal("classify: -company must be positive")
		}
		resolver, err := llmboot.Resolver(cfg, db)
		if err != nil {
			log.Fatalf("classify: resolver: %v", err)
		}
		registry, err := prompts.NewRegistry()
		if err != nil {
			log.Fatalf("classify: load prompts: %v", err)
		}
		repository := classifymysql.New(db)
		workflow, err := classify.New(classify.Config{
			Registry: registry,
			Source:   repository,
			Lookup:   repository,
			Sink:     repository,
			Resolve:  resolver.ResolveFor,
			Review:   reviewmysql.NewOutboxWriter(db),
			Model:    cfg.LLM.DefaultModel,
		})
		if err != nil {
			log.Fatalf("classify: build workflow: %v", err)
		}
		result, err := workflow.Run(ctx, classify.Options{
			CompanyID: *company,
			Limit:     *limit,
		})
		if err != nil {
			log.Fatalf("classify: %v", err)
		}
		fmt.Printf("classify complete: company=%d examined=%d classified=%d failed=%d\n", *company, result.Examined, result.Classified, result.Failed)
	default:
		log.Fatalf("unknown task %q", *task)
	}
}

func parseSince(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, errors.New("-since must be RFC3339 or YYYY-MM-DD")
	}
	return parsed, nil
}
