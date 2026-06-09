package ports

import (
	"context"
	"time"
)

type Job struct {
	Type      string
	CompanyID int64
	TraceID   string
	Payload   []byte
	Attempt   int
	Deadline  time.Time
	UniqueFor time.Duration
}

type Queue interface {
	Enqueue(ctx context.Context, j Job) error
	EnqueueIn(ctx context.Context, j Job, delay time.Duration) error
}
