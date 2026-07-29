package main

import (
	"database/sql"

	"github.com/my/app/internal/infra/tenant"
	"github.com/my/app/internal/shared/config"
)

const (
	commandIndexKB      = "index-kb"
	commandReembed      = "reembed"
	commandBatchSubmit  = "embed-batch-submit"
	commandBatchCollect = "embed-batch-collect"
	commandBatchRun     = "embed-batch-run"
)

type commandOptions struct {
	task                string
	company             int64
	collection          string
	batchSize           int
	limit               int
	provider            string
	dim                 int
	model               string
	baseURL             string
	embedRPM            float64
	embedRetries        int
	companies           string
	batchJob            string
	maxPerJob           int
	waveSize            int
	dryRun              bool
	maxChunks           int
	allowLargeRemoteRun bool
	chunkRunes          int
}

type commandDependencies struct {
	cfg config.Config
	db  *sql.DB
	qdb tenant.Querier
}
