package symptom

import (
	"context"
	"testing"

	"github.com/my/app/internal/domain/ports"
)

type fakeCatalog struct {
	syms   []Symptom
	err    error
	called int
}

func (f *fakeCatalog) Symptoms(ctx context.Context, companyID int64) ([]Symptom, error) {
	f.called++
	return f.syms, f.err
}

type fakeProvider struct {
	json   string
	called int
}

func (f *fakeProvider) Generate(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	return ports.Completion{}, nil
}

func (f *fakeProvider) GenerateJSON(ctx context.Context, p ports.Prompt) (ports.Completion, error) {
	f.called++
	return ports.Completion{Text: f.json}, nil
}

func (f *fakeProvider) Stream(ctx context.Context, p ports.Prompt) (<-chan ports.Completion, error) {
	return nil, nil
}

func newMatcher(t *testing.T, cat *fakeCatalog, prov *fakeProvider, threshold float64) *Matcher {
	t.Helper()
	m, err := New(Config{
		Catalog:   cat,
		Resolve:   func(ctx context.Context, companyID int64) (ports.LLMProvider, error) { return prov, nil },
		Threshold: threshold,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return m
}

func TestMatchReturnsKnownSymptomAboveThreshold(t *testing.T) {
	cat := &fakeCatalog{syms: []Symptom{{ID: 1, Name: "wifi down"}, {ID: 2, Name: "ups failure"}}}
	prov := &fakeProvider{json: `{"symptom_id":2,"confidence":0.9}`}
	m := newMatcher(t, cat, prov, 0.70)

	got, err := m.Match(context.Background(), 7, "the UPS won't back up power")
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if got.SymptomID != 2 || got.Name != "ups failure" || got.Confidence != 0.9 {
		t.Fatalf("got %+v, want {2 ups failure 0.9}", got)
	}
}

func TestMatchBelowThresholdReturnsNone(t *testing.T) {
	cat := &fakeCatalog{syms: []Symptom{{ID: 1, Name: "wifi down"}}}
	prov := &fakeProvider{json: `{"symptom_id":1,"confidence":0.5}`}
	m := newMatcher(t, cat, prov, 0.70)

	got, err := m.Match(context.Background(), 7, "something vague")
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if got.SymptomID != 0 {
		t.Fatalf("got %+v, want no match (below threshold)", got)
	}
}

func TestMatchUnknownSymptomIDReturnsNone(t *testing.T) {
	cat := &fakeCatalog{syms: []Symptom{{ID: 1, Name: "wifi down"}}}
	prov := &fakeProvider{json: `{"symptom_id":99,"confidence":0.95}`}
	m := newMatcher(t, cat, prov, 0.70)

	got, err := m.Match(context.Background(), 7, "anything")
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if got.SymptomID != 0 {
		t.Fatalf("got %+v, want no match (id not in catalog)", got)
	}
}

func TestMatchEmptyCatalogSkipsLLM(t *testing.T) {
	cat := &fakeCatalog{syms: nil}
	prov := &fakeProvider{json: `{"symptom_id":1,"confidence":0.9}`}
	m := newMatcher(t, cat, prov, 0.70)

	got, err := m.Match(context.Background(), 7, "the UPS won't back up power")
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if got.SymptomID != 0 {
		t.Fatalf("got %+v, want no match (empty catalog)", got)
	}
	if prov.called != 0 {
		t.Fatalf("provider called %d times, want 0 (no symptoms to match)", prov.called)
	}
}

func TestMatchEmptyMessageSkipsEverything(t *testing.T) {
	cat := &fakeCatalog{syms: []Symptom{{ID: 1, Name: "wifi down"}}}
	prov := &fakeProvider{json: `{"symptom_id":1,"confidence":0.9}`}
	m := newMatcher(t, cat, prov, 0.70)

	got, err := m.Match(context.Background(), 7, "   ")
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if got.SymptomID != 0 || cat.called != 0 || prov.called != 0 {
		t.Fatalf("empty message should short-circuit: got %+v cat=%d prov=%d", got, cat.called, prov.called)
	}
}
