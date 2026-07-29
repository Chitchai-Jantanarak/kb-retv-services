package main

import (
	"github.com/hibiken/asynq"

	"github.com/my/app/internal/application/services/tickets"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/config"
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

func buildTaskMux(cfg config.Config, classifyQueue ports.Queue) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskReplyLine, makeHandler(stubHandler("reply:line")))
	mux.HandleFunc(TaskReplyEmail, makeHandler(stubHandler("reply:email")))
	mux.HandleFunc(TaskIngestReport, makeHandler(buildIngestHandler(cfg)))
	mux.HandleFunc(TaskExtractBatch, makeHandler(buildExtractHandler(cfg)))
	mux.HandleFunc(TaskClusterWeekly, makeHandler(stubHandler("cluster:weekly")))

	promoteHandler := buildPromoteHandler(cfg)
	mux.HandleFunc(TaskPromoteKB, makeHandler(promoteHandler))
	mux.HandleFunc(TaskPromoteSymptom, makeHandler(promoteHandler))
	mux.HandleFunc(TaskPromoteSubject, makeHandler(promoteHandler))

	mux.HandleFunc(TaskOutboxFlush, makeHandler(buildOutboxHandler(cfg)))
	mux.HandleFunc(TaskEmbeddingRefresh, makeHandler(buildRefreshHandler(cfg)))
	mux.HandleFunc(tickets.TaskCreate, makeHandler(buildTicketCreateHandler(cfg, classifyQueue)))

	return mux
}
