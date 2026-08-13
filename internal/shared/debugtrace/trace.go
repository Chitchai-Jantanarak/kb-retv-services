package debugtrace

import (
	"context"
	"sync"
	"time"
)

// Event is a sanitized, debug-only execution event. Callers must never place
// credentials, raw SQL, tenant identifiers, or unredacted payloads in Context.
type Event struct {
	Stage      string
	State      string
	Label      string
	Context    map[string]string
	DurationMS int64
	ErrorCode  string
	Error      string
}

type collectorKey struct{}

type Collector struct {
	mu     sync.Mutex
	events []Event
}

func WithCollector(ctx context.Context) (context.Context, *Collector) {
	collector := &Collector{}
	return context.WithValue(ctx, collectorKey{}, collector), collector
}

func Enabled(ctx context.Context) bool {
	collector, _ := ctx.Value(collectorKey{}).(*Collector)
	return collector != nil
}

func Add(ctx context.Context, event Event) {
	collector, _ := ctx.Value(collectorKey{}).(*Collector)
	if collector == nil {
		return
	}
	event.Context = cloneContext(event.Context)
	collector.mu.Lock()
	collector.events = append(collector.events, event)
	collector.mu.Unlock()
}

func (c *Collector) Events() []Event {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Event, len(c.events))
	for i, event := range c.events {
		event.Context = cloneContext(event.Context)
		out[i] = event
	}
	return out
}

func Since(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

func cloneContext(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
