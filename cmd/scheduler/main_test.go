package main

import (
	"testing"
	"time"

	infraasynq "github.com/my/app/internal/infra/asynq"
)

func TestScheduledCompanyTaskIsWorkerDecodable(t *testing.T) {
	task, opts, err := scheduledCompanyTask("embedding:refresh", 42, 5*time.Hour)
	if err != nil {
		t.Fatalf("scheduledCompanyTask: %v", err)
	}
	if task.Type() != "embedding:refresh" {
		t.Fatalf("task.Type = %q, want embedding:refresh", task.Type())
	}
	if len(opts) != 1 {
		t.Fatalf("opts = %d, want one unique option", len(opts))
	}

	job, err := infraasynq.ExtractJob(task)
	if err != nil {
		t.Fatalf("ExtractJob: %v", err)
	}
	if job.Type != "embedding:refresh" || job.CompanyID != 42 {
		t.Fatalf("job = %+v, want embedding:refresh for company 42", job)
	}
}

func TestScheduledCompanyTaskRejectsMissingCompany(t *testing.T) {
	if _, _, err := scheduledCompanyTask("cluster:weekly", 0, time.Hour); err == nil {
		t.Fatal("scheduledCompanyTask err = nil, want missing company error")
	}
}
