package tickets

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/my/app/internal/application/dto"
	"github.com/my/app/internal/domain/ports"
)

type captureQueue struct {
	jobs []ports.Job
	err  error
}

func (q *captureQueue) Enqueue(_ context.Context, j ports.Job) error {
	if q.err != nil {
		return q.err
	}
	q.jobs = append(q.jobs, j)
	return nil
}

func (q *captureQueue) EnqueueIn(_ context.Context, _ ports.Job, _ time.Duration) error {
	return errors.New("not used")
}

func TestNewEnqueuerRejectsNilQueue(t *testing.T) {
	_, err := NewEnqueuer(nil)
	if err == nil || !strings.Contains(err.Error(), "queue is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestEnqueueTicketProducesJob(t *testing.T) {
	q := &captureQueue{}
	e, err := NewEnqueuer(q)
	if err != nil {
		t.Fatalf("NewEnqueuer: %v", err)
	}
	err = e.EnqueueTicket(context.Background(), 7, 100, 200, "Uabc", dto.InboundMessageRequest{
		Channel:           "line",
		ExternalMessageID: "m-1",
		CustomerID:        "Uabc",
		Body:              "hi",
	})
	if err != nil {
		t.Fatalf("EnqueueTicket: %v", err)
	}
	if len(q.jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(q.jobs))
	}
	j := q.jobs[0]
	if j.Type != TaskCreate || j.CompanyID != 7 {
		t.Fatalf("job header wrong: %+v", j)
	}
	var p Payload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.MessageID != 200 || p.ConversationID != 100 || p.Channel != "line" || p.ExternalMessageID != "m-1" {
		t.Fatalf("payload wrong: %+v", p)
	}
	if p.SenderExternal != "Uabc" {
		t.Fatalf("sender_external = %q, want Uabc", p.SenderExternal)
	}
}

func TestEnqueueTicketBubblesQueueError(t *testing.T) {
	q := &captureQueue{err: errors.New("redis down")}
	e, err := NewEnqueuer(q)
	if err != nil {
		t.Fatalf("NewEnqueuer: %v", err)
	}
	err = e.EnqueueTicket(context.Background(), 7, 100, 200, "Uabc", dto.InboundMessageRequest{
		Channel:           "line",
		ExternalMessageID: "m-1",
	})
	if err == nil || !strings.Contains(err.Error(), "redis down") {
		t.Fatalf("err = %v", err)
	}
}
