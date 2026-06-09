package classify

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/my/app/internal/ai/prompts"
	"github.com/my/app/internal/domain/ports"
)

type captureReview struct {
	mu    sync.Mutex
	items []ports.ReviewItem
}

func (c *captureReview) Enqueue(_ context.Context, item ports.ReviewItem) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = append(c.items, item)
	return nil
}

type stubProvider struct {
	mu         sync.Mutex
	stage1Resp string
	stage1Err  error
	stage2Resp string
	stage2Err  error
	calls      int
}

func (p *stubProvider) Generate(_ context.Context, _ ports.Prompt) (ports.Completion, error) {
	return ports.Completion{}, errors.New("not used")
}

func (p *stubProvider) GenerateJSON(_ context.Context, _ ports.Prompt) (ports.Completion, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.calls%2 == 1 {
		return ports.Completion{Text: p.stage1Resp}, p.stage1Err
	}
	return ports.Completion{Text: p.stage2Resp}, p.stage2Err
}

func (p *stubProvider) Stream(_ context.Context, _ ports.Prompt) (<-chan ports.Completion, error) {
	return nil, errors.New("not used")
}

type stubSource struct {
	reports []ReportRecord
	err     error
}

func (s *stubSource) UnclassifiedReports(_ context.Context, companyID int64, _ int) ([]ReportRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([]ReportRecord, 0, len(s.reports))
	for _, r := range s.reports {
		if r.CompanyID == companyID {
			out = append(out, r)
		}
	}
	return out, nil
}

type stubLookup struct {
	problemTypeID int64
	severityID    int64
	knownErr      error
	problemErr    error
	severityErr   error
	subjectErr    error
	symptomErr    error
}

func (l *stubLookup) ProblemTypeIDByCode(_ context.Context, _ string) (int64, error) {
	if l.problemErr != nil {
		return 0, l.problemErr
	}
	return l.problemTypeID, nil
}

func (l *stubLookup) SeverityIDByCode(_ context.Context, _ string) (int64, error) {
	if l.severityErr != nil {
		return 0, l.severityErr
	}
	return l.severityID, nil
}

func (l *stubLookup) KnownSymptoms(_ context.Context, _ int64, _ int) ([]string, error) {
	if l.knownErr != nil {
		return nil, l.knownErr
	}
	return []string{"printer_offline"}, nil
}

func (l *stubLookup) UpsertSubject(_ context.Context, _ int64, _ string) (int64, error) {
	if l.subjectErr != nil {
		return 0, l.subjectErr
	}
	return 77, nil
}

func (l *stubLookup) UpsertSymptom(_ context.Context, _ int64, _ string, _ bool) (int64, error) {
	if l.symptomErr != nil {
		return 0, l.symptomErr
	}
	return 88, nil
}

type stubSink struct {
	mu       sync.Mutex
	received []Classification
	err      error
}

func (s *stubSink) WriteClassification(_ context.Context, c Classification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.received = append(s.received, c)
	return nil
}

func newRegistry(t *testing.T) *prompts.Registry {
	t.Helper()
	r, err := prompts.NewRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return r
}

func TestNewFailureCases(t *testing.T) {
	reg := newRegistry(t)
	good := Config{
		Registry: reg,
		Source:   &stubSource{},
		Lookup:   &stubLookup{},
		Sink:     &stubSink{},
		Resolve:  func(_ context.Context, _ int64) (ports.LLMProvider, error) { return nil, nil },
	}
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{name: "no_registry", mutate: func(c *Config) { c.Registry = nil }, wantSub: "registry is required"},
		{name: "no_source", mutate: func(c *Config) { c.Source = nil }, wantSub: "source is required"},
		{name: "no_lookup", mutate: func(c *Config) { c.Lookup = nil }, wantSub: "lookup is required"},
		{name: "no_sink", mutate: func(c *Config) { c.Sink = nil }, wantSub: "sink is required"},
		{name: "no_resolve", mutate: func(c *Config) { c.Resolve = nil }, wantSub: "resolver is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := good
			tc.mutate(&cfg)
			_, err := New(cfg)
			if err == nil {
				t.Fatalf("err = nil, want %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %q, want sub %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestRunRejectsBadCompanyID(t *testing.T) {
	wf, err := New(Config{
		Registry: newRegistry(t),
		Source:   &stubSource{},
		Lookup:   &stubLookup{},
		Sink:     &stubSink{},
		Resolve:  func(_ context.Context, _ int64) (ports.LLMProvider, error) { return nil, nil },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = wf.Run(context.Background(), Options{CompanyID: 0})
	if err == nil || !strings.Contains(err.Error(), "company_id must be positive") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunCrossTenantRecordAborts(t *testing.T) {
	src := &stubSource{}
	src.reports = []ReportRecord{{ID: 1, CompanyID: 99, Title: "leaked"}}
	wf, err := New(Config{
		Registry: newRegistry(t),
		Source:   &fixedSource{out: src.reports},
		Lookup:   &stubLookup{problemTypeID: 1, severityID: 1},
		Sink:     &stubSink{},
		Resolve: func(_ context.Context, _ int64) (ports.LLMProvider, error) {
			return &stubProvider{stage1Resp: `{}`, stage2Resp: `{}`}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = wf.Run(context.Background(), Options{CompanyID: 7, Limit: 5})
	if err == nil || !strings.Contains(err.Error(), "belongs to company") {
		t.Fatalf("err = %v, want cross-tenant rejection", err)
	}
}

type fixedSource struct{ out []ReportRecord }

func (s *fixedSource) UnclassifiedReports(_ context.Context, _ int64, _ int) ([]ReportRecord, error) {
	return s.out, nil
}

func TestRunHappyPathWritesClassification(t *testing.T) {
	src := &stubSource{reports: []ReportRecord{
		{ID: 100, CompanyID: 7, Title: "Printer broken", Body: "printer offline"},
	}}
	sink := &stubSink{}
	wf, err := New(Config{
		Registry: newRegistry(t),
		Source:   src,
		Lookup:   &stubLookup{problemTypeID: 2, severityID: 3},
		Sink:     sink,
		Resolve: func(_ context.Context, _ int64) (ports.LLMProvider, error) {
			return &stubProvider{
				stage1Resp: `{"subject":"Printer HQ","problem_type":"hardware","confidence":0.8,"rationale":"x"}`,
				stage2Resp: `{"severity":"high","symptom":"printer_offline","symptom_is_new":false,"confidence":0.9}`,
			}, nil
		},
		Model: "test-model",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := wf.Run(context.Background(), Options{CompanyID: 7, Limit: 5})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Classified != 1 || res.Failed != 0 || res.Examined != 1 {
		t.Fatalf("result = %+v, want 1 classified", res)
	}
	if len(sink.received) != 1 {
		t.Fatalf("sink.received = %d, want 1", len(sink.received))
	}
	got := sink.received[0]
	if got.ReportID != 100 || got.CompanyID != 7 {
		t.Fatalf("report identity wrong: %+v", got)
	}
	if got.ProblemTypeID != 2 || got.SeverityID != 3 {
		t.Fatalf("lookup ids: %+v", got)
	}
	if got.SubjectNodeID != 77 || got.SymptomNodeID != 88 {
		t.Fatalf("upsert ids: %+v", got)
	}
	if got.Model != "test-model" {
		t.Fatalf("model = %q", got.Model)
	}
}

func TestRunCountsFailures(t *testing.T) {
	cases := []struct {
		name       string
		stage1Resp string
		stage1Err  error
		stage2Resp string
		stage2Err  error
		lookupErr  error
	}{
		{name: "stage1_error", stage1Err: errors.New("upstream 500"), stage2Resp: "{}"},
		{name: "stage1_bad_json", stage1Resp: "not-json", stage2Resp: "{}"},
		{name: "stage2_error", stage1Resp: `{"subject":"x","problem_type":"hardware","confidence":0.5}`, stage2Err: errors.New("upstream 500")},
		{name: "stage2_bad_json", stage1Resp: `{"subject":"x","problem_type":"hardware","confidence":0.5}`, stage2Resp: "not-json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := &stubSource{reports: []ReportRecord{{ID: 1, CompanyID: 7, Title: "T", Body: "B"}}}
			wf, err := New(Config{
				Registry: newRegistry(t),
				Source:   src,
				Lookup:   &stubLookup{problemTypeID: 1, severityID: 1},
				Sink:     &stubSink{},
				Resolve: func(_ context.Context, _ int64) (ports.LLMProvider, error) {
					return &stubProvider{
						stage1Resp: tc.stage1Resp,
						stage1Err:  tc.stage1Err,
						stage2Resp: tc.stage2Resp,
						stage2Err:  tc.stage2Err,
					}, nil
				},
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			res, err := wf.Run(context.Background(), Options{CompanyID: 7, Limit: 5})
			if err != nil {
				t.Fatalf("Run err = %v, want nil (failures counted, not returned)", err)
			}
			if res.Failed != 1 || res.Classified != 0 {
				t.Fatalf("result = %+v, want 1 failed", res)
			}
		})
	}
}

func TestRunEnqueuesNewSymptomForReview(t *testing.T) {
	src := &stubSource{reports: []ReportRecord{
		{ID: 200, CompanyID: 7, Title: "Weird new thing", Body: "never seen before"},
	}}
	rev := &captureReview{}
	wf, err := New(Config{
		Registry: newRegistry(t),
		Source:   src,
		Lookup:   &stubLookup{problemTypeID: 1, severityID: 2},
		Sink:     &stubSink{},
		Review:   rev,
		Resolve: func(_ context.Context, _ int64) (ports.LLMProvider, error) {
			return &stubProvider{
				stage1Resp: `{"subject":"Robot","problem_type":"hardware","confidence":0.6}`,
				stage2Resp: `{"severity":"medium","symptom":"robot_dancing","symptom_is_new":true,"confidence":0.7}`,
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = wf.Run(context.Background(), Options{CompanyID: 7, Limit: 1})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	rev.mu.Lock()
	defer rev.mu.Unlock()
	if len(rev.items) != 1 {
		t.Fatalf("review items = %d, want 1", len(rev.items))
	}
	if rev.items[0].Kind != ports.ReviewKindSymptomPropose || rev.items[0].CompanyID != 7 {
		t.Fatalf("review item = %+v", rev.items[0])
	}
}

func TestRunDoesNotEnqueueExistingSymptom(t *testing.T) {
	src := &stubSource{reports: []ReportRecord{
		{ID: 200, CompanyID: 7, Title: "Known thing", Body: "seen this"},
	}}
	rev := &captureReview{}
	wf, err := New(Config{
		Registry: newRegistry(t),
		Source:   src,
		Lookup:   &stubLookup{problemTypeID: 1, severityID: 2},
		Sink:     &stubSink{},
		Review:   rev,
		Resolve: func(_ context.Context, _ int64) (ports.LLMProvider, error) {
			return &stubProvider{
				stage1Resp: `{"subject":"X","problem_type":"software","confidence":0.5}`,
				stage2Resp: `{"severity":"low","symptom":"printer_offline","symptom_is_new":false,"confidence":0.9}`,
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = wf.Run(context.Background(), Options{CompanyID: 7, Limit: 1})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	rev.mu.Lock()
	defer rev.mu.Unlock()
	if len(rev.items) != 0 {
		t.Fatalf("review items = %d, want 0 (existing symptom)", len(rev.items))
	}
}

func TestRunResolveErrorIsFatal(t *testing.T) {
	src := &stubSource{reports: []ReportRecord{{ID: 1, CompanyID: 7, Title: "T", Body: "B"}}}
	wf, err := New(Config{
		Registry: newRegistry(t),
		Source:   src,
		Lookup:   &stubLookup{problemTypeID: 1, severityID: 1},
		Sink:     &stubSink{},
		Resolve:  func(_ context.Context, _ int64) (ports.LLMProvider, error) { return nil, errors.New("no provider") },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = wf.Run(context.Background(), Options{CompanyID: 7})
	if err == nil || !strings.Contains(err.Error(), "resolve provider") {
		t.Fatalf("err = %v", err)
	}
}
