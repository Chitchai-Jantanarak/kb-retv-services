package asynq

import (
	"testing"
	"time"

	"github.com/my/app/internal/domain/ports"
)

func TestJobToTaskAppendsOptionsBasedOnJob(t *testing.T) {
	deadline := time.Now().Add(time.Hour)
	cases := []struct {
		name     string
		job      ports.Job
		wantOpts int
	}{
		{name: "no_options", job: ports.Job{Type: "x", CompanyID: 1}, wantOpts: 0},
		{name: "deadline_only", job: ports.Job{Type: "x", CompanyID: 1, Deadline: deadline}, wantOpts: 1},
		{name: "retry_only", job: ports.Job{Type: "x", CompanyID: 1, Attempt: 3}, wantOpts: 1},
		{name: "unique_only", job: ports.Job{Type: "x", CompanyID: 1, UniqueFor: 10 * time.Minute}, wantOpts: 1},
		{name: "all_three", job: ports.Job{Type: "x", CompanyID: 1, Deadline: deadline, Attempt: 3, UniqueFor: 10 * time.Minute}, wantOpts: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task, opts, err := jobToTask(tc.job)
			if err != nil {
				t.Fatalf("jobToTask err = %v", err)
			}
			if task.Type() != tc.job.Type {
				t.Fatalf("task.Type = %q, want %q", task.Type(), tc.job.Type)
			}
			if len(opts) != tc.wantOpts {
				t.Fatalf("opts len = %d, want %d", len(opts), tc.wantOpts)
			}
		})
	}
}

func TestJobToTaskRoundTrip(t *testing.T) {
	original := ports.Job{
		Type:      "ingest:report",
		CompanyID: 42,
		TraceID:   "trace-123",
		Payload:   []byte(`{"limit":5}`),
	}
	task, _, err := jobToTask(original)
	if err != nil {
		t.Fatalf("jobToTask: %v", err)
	}
	got, err := ExtractJob(task)
	if err != nil {
		t.Fatalf("ExtractJob: %v", err)
	}
	if got.Type != original.Type || got.CompanyID != original.CompanyID || got.TraceID != original.TraceID {
		t.Fatalf("round trip mismatch: %+v vs %+v", got, original)
	}
}
