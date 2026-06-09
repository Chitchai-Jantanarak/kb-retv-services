package ports

import "context"

type KnowledgeGapRecorder interface {
	Record(ctx context.Context, companyID int64, queryText string) error
}
