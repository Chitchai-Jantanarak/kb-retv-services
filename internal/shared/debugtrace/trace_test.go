package debugtrace

import (
	"context"
	"testing"
)

func TestCollectorCopiesEventContext(t *testing.T) {
	if Enabled(context.Background()) {
		t.Fatal("background context must not enable debug tracing")
	}
	ctx, collector := WithCollector(context.Background())
	if !Enabled(ctx) {
		t.Fatal("collector context must enable debug tracing")
	}
	metadata := map[string]string{"operation": "SELECT"}
	Add(ctx, Event{Stage: "mysql.query", Context: metadata})
	metadata["operation"] = "MUTATED"

	events := collector.Events()
	if len(events) != 1 || events[0].Context["operation"] != "SELECT" {
		t.Fatalf("events = %#v", events)
	}
	events[0].Context["operation"] = "CHANGED"
	if collector.Events()[0].Context["operation"] != "SELECT" {
		t.Fatal("Events must return a defensive copy")
	}
}
