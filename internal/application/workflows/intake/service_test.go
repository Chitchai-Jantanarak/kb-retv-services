package intake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/my/app/internal/application/workflows/intake"
)

type fakeSink struct {
	conversationID int64
	result         intake.Result
	err            error
	calls          int
}

func (f *fakeSink) WriteCompleteness(_ context.Context, conversationID int64, r intake.Result) error {
	f.calls++
	f.conversationID = conversationID
	f.result = r
	return f.err
}

func TestServiceAssessPersistsResult(t *testing.T) {
	provider := &fakeProvider{json: `{"problem_detail":"stuck","product":"Bella Bot"}`}
	sink := &fakeSink{}
	svc := intake.NewService(newExtractor(t, provider, intake.WithProducts(fakeProducts{products: []string{"Bella Bot"}})), sink, nil)

	got, err := svc.Assess(context.Background(), 7, 42, "cust@x.com", "Robot stuck", "will not move")
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}
	if got.Status != intake.StatusReady {
		t.Fatalf("Status = %q, want ready", got.Status)
	}
	if sink.calls != 1 || sink.conversationID != 42 {
		t.Fatalf("sink calls = %d, conversation = %d", sink.calls, sink.conversationID)
	}
	if sink.result.Status != intake.StatusReady {
		t.Fatalf("persisted status = %q", sink.result.Status)
	}
}

func TestServiceAssessReturnsErrorWhenExtractionFails(t *testing.T) {
	provider := &fakeProvider{err: errors.New("llm down")}
	sink := &fakeSink{}
	svc := intake.NewService(newExtractor(t, provider, intake.WithProducts(fakeProducts{products: []string{"Bella Bot"}})), sink, nil)

	got, err := svc.Assess(context.Background(), 7, 42, "cust@x.com", "s", "b")
	if err == nil {
		t.Fatal("Assess() error = nil, want extraction error")
	}
	if got.Status != intake.StatusUnknown {
		t.Fatalf("Status = %q, want unknown", got.Status)
	}
	if sink.calls != 0 {
		t.Fatalf("sink must not be written on extraction failure (calls = %d)", sink.calls)
	}
}

func TestServiceAssessNilServiceIsUnknown(t *testing.T) {
	var svc *intake.Service
	got, err := svc.Assess(context.Background(), 7, 42, "cust@x.com", "s", "b")
	if err != nil {
		t.Fatalf("Assess() error = %v", err)
	}
	if got.Status != intake.StatusUnknown {
		t.Fatalf("Status = %q, want unknown", got.Status)
	}
}
