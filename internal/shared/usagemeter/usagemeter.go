// Package usagemeter accumulates language-model token usage across the stages
// of a single request.
//
// The reply pipeline calls a provider up to four times — rerank, corrective
// check, generate, critique — and each call returns its own usage. Without an
// accumulator that usage is discarded at the end of each stage, so the action
// record for the request carries no token counts and the cost of the pipeline
// cannot be measured after the fact.
//
// The accumulator travels in the context rather than through the signatures of
// the four stages, mirroring shared/debugtrace, because threading a parameter
// through five call sites to carry a diagnostic would put measurement concerns
// into the pipeline's interfaces.
//
// A request without a meter in its context records nothing and costs nothing;
// callers that do not opt in are unaffected.
package usagemeter

import (
	"context"
	"maps"
	"sync"

	"github.com/my/app/internal/domain/ports"
)

type meterKey struct{}

// Meter accumulates usage across concurrent stages of one request.
type Meter struct {
	mu     sync.Mutex
	input  int
	output int
	calls  int
	byName map[string]ports.TokenUsage
}

// With returns a context carrying a new meter, and the meter.
func With(ctx context.Context) (context.Context, *Meter) {
	m := &Meter{byName: make(map[string]ports.TokenUsage)}
	return context.WithValue(ctx, meterKey{}, m), m
}

// From returns the meter in ctx, or nil when none was installed.
func From(ctx context.Context) *Meter {
	m, _ := ctx.Value(meterKey{}).(*Meter)
	return m
}

// Add records one provider call against the meter in ctx. It is safe to call
// when no meter is installed, which is the ordinary production path.
func Add(ctx context.Context, stage string, usage ports.TokenUsage) {
	m := From(ctx)
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.input += usage.Input
	m.output += usage.Output
	m.calls++
	prev := m.byName[stage]
	prev.Input += usage.Input
	prev.Output += usage.Output
	prev.Total += usage.Total
	m.byName[stage] = prev
}

// Totals returns the accumulated input tokens, output tokens, and the number
// of provider calls made. A nil meter reports zeroes.
func (m *Meter) Totals() (input, output, calls int) {
	if m == nil {
		return 0, 0, 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.input, m.output, m.calls
}

// ByStage returns a copy of the per-stage breakdown, which is what makes the
// measurement useful: knowing a request cost 3,000 tokens matters less than
// knowing which stage spent them.
func (m *Meter) ByStage() map[string]ports.TokenUsage {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]ports.TokenUsage, len(m.byName))
	maps.Copy(out, m.byName)
	return out
}
