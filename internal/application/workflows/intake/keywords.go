package intake

import "context"

type KeywordSource interface {
	IntentKeywords(ctx context.Context, companyID int64) ([]string, error)
}

type ThresholdSource interface {
	PromoteThreshold(ctx context.Context, companyID int64) (int, error)
}
