package asynq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"

	"github.com/my/app/internal/domain/ports"
)

// Queue implements ports.Queue using asynq (Redis-backed).
type Queue struct {
	client *asynq.Client
}

type Config struct {
	RedisURL string
}

func NewQueue(cfg Config) (*Queue, error) {
	opt, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("asynq: parse redis url: %w", err)
	}
	client := asynq.NewClient(opt)
	return &Queue{client: client}, nil
}

func (q *Queue) Close() error {
	return q.client.Close()
}

func (q *Queue) Enqueue(ctx context.Context, j ports.Job) error {
	task, opts, err := jobToTask(j)
	if err != nil {
		return err
	}
	_, err = q.client.EnqueueContext(ctx, task, opts...)
	return err
}

func (q *Queue) EnqueueIn(ctx context.Context, j ports.Job, delay time.Duration) error {
	task, opts, err := jobToTask(j)
	if err != nil {
		return err
	}
	opts = append(opts, asynq.ProcessIn(delay))
	_, err = q.client.EnqueueContext(ctx, task, opts...)
	return err
}

type taskPayload struct {
	CompanyID int64           `json:"company_id"`
	TraceID   string          `json:"trace_id,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

func jobToTask(j ports.Job) (*asynq.Task, []asynq.Option, error) {
	tp := taskPayload{
		CompanyID: j.CompanyID,
		TraceID:   j.TraceID,
		Payload:   j.Payload,
	}
	data, err := json.Marshal(tp)
	if err != nil {
		return nil, nil, fmt.Errorf("asynq: marshal job: %w", err)
	}

	var opts []asynq.Option
	if !j.Deadline.IsZero() {
		opts = append(opts, asynq.Deadline(j.Deadline))
	}
	if j.Attempt > 0 {
		opts = append(opts, asynq.MaxRetry(j.Attempt))
	}
	if j.UniqueFor > 0 {
		opts = append(opts, asynq.Unique(j.UniqueFor))
	}

	return asynq.NewTask(j.Type, data), opts, nil
}

func TaskForJob(j ports.Job) (*asynq.Task, []asynq.Option, error) {
	return jobToTask(j)
}

func ExtractJob(task *asynq.Task) (ports.Job, error) {
	var tp taskPayload
	if err := json.Unmarshal(task.Payload(), &tp); err != nil {
		return ports.Job{}, fmt.Errorf("asynq: unmarshal job: %w", err)
	}
	return ports.Job{
		Type:      task.Type(),
		CompanyID: tp.CompanyID,
		TraceID:   tp.TraceID,
		Payload:   tp.Payload,
	}, nil
}

var _ ports.Queue = (*Queue)(nil)
