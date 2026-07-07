package intent

import (
	"context"
	"errors"
	"testing"
)

type stubEmbedder struct {
	vecs  map[string][]float32
	calls [][]string
	err   error
}

func (s *stubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	s.calls = append(s.calls, texts)
	if s.err != nil {
		return nil, s.err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v, ok := s.vecs[t]
		if !ok {
			return nil, errors.New("stubEmbedder: no vector for " + t)
		}
		out[i] = v
	}
	return out, nil
}

type stubScorer struct {
	score float64
	err   error
	calls int
}

func (s *stubScorer) TopScore(_ context.Context, _ string) (float64, error) {
	s.calls++
	if s.err != nil {
		return 0, s.err
	}
	return s.score, nil
}

func newFixture(t *testing.T, score float64, scorerErr error) (*Router, *stubEmbedder, *stubScorer) {
	t.Helper()

	emb := &stubEmbedder{vecs: map[string][]float32{
		"ติดต่อเจ้าหน้าที่":     {1, 0, 0, 0},
		"ตรวจสอบสถานะเคส":       {0, 1, 0, 0},
		"เปิดเคสใหม่":            {0, 0, 1, 0},
		"ใช้งานระบบนี้อย่างไร":  {0, 0, 0, 1},

		"ขอสายเจ้าหน้าที่":     {1, 0, 0, 0},
		"เช็คสถานะเคส":          {0, 1, 0, 0},
		"แจ้งเปิดเคส":           {0, 0, 1, 0},
		"ถามวิธีใช้ระบบ":        {0, 0, 0, 1},
		"หุ่นยนต์พังแก้ยังไง":  {0, 0, 0, 0, 1},
	}}
	scorer := &stubScorer{score: score, err: scorerErr}

	r, err := New(context.Background(), emb, scorer,
		WithAnchors(map[Intent][]string{
			Handoff:        {"ติดต่อเจ้าหน้าที่"},
			CaseStatus:     {"ตรวจสอบสถานะเคส"},
			OpenCase:       {"เปิดเคสใหม่"},
			GeneralSupport: {"ใช้งานระบบนี้อย่างไร"},
		}),
		WithThresholds(0.4, 0.6),
		WithRetrievalMin(1.0),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return r, emb, scorer
}

func TestRouterRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		query            string
		retrievalScore   float64
		wantIntent       Intent
		wantReason       Reason
		wantScorerCalled bool
	}{
		{name: "handoff action anchor skips retrieval", query: "ขอสายเจ้าหน้าที่", wantIntent: Handoff, wantReason: ReasonActionAnchor, wantScorerCalled: false},
		{name: "case status action anchor", query: "เช็คสถานะเคส", wantIntent: CaseStatus, wantReason: ReasonActionAnchor, wantScorerCalled: false},
		{name: "open case action anchor", query: "แจ้งเปิดเคส", wantIntent: OpenCase, wantReason: ReasonActionAnchor, wantScorerCalled: false},
		{name: "kb question rescued by retrieval hit", query: "หุ่นยนต์พังแก้ยังไง", retrievalScore: 5.0, wantIntent: KBSearch, wantReason: ReasonRetrievalHit, wantScorerCalled: true},
		{name: "in domain but no kb hit", query: "ถามวิธีใช้ระบบ", retrievalScore: 0.2, wantIntent: GeneralSupport, wantReason: ReasonInDomainNoKB, wantScorerCalled: true},
		{name: "off domain below floor and no kb", query: "หุ่นยนต์พังแก้ยังไง", retrievalScore: 0.0, wantIntent: OffDomain, wantReason: ReasonOffDomainFloor, wantScorerCalled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r, _, scorer := newFixture(t, tt.retrievalScore, nil)
			got, err := r.Route(context.Background(), tt.query)
			if err != nil {
				t.Fatalf("Route() error = %v", err)
			}
			if got.Intent != tt.wantIntent {
				t.Errorf("Intent = %q, want %q", got.Intent, tt.wantIntent)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if called := scorer.calls > 0; called != tt.wantScorerCalled {
				t.Errorf("scorer called = %v, want %v", called, tt.wantScorerCalled)
			}
		})
	}
}

func TestRouterPropagatesEmbedError(t *testing.T) {
	t.Parallel()

	r, _, scorer := newFixture(t, 0, nil)
	if _, err := r.Route(context.Background(), "ไม่มีเวกเตอร์"); err == nil {
		t.Fatal("Route() error = nil, want embed error")
	}
	if scorer.calls != 0 {
		t.Errorf("scorer called %d times, want 0 when embed fails", scorer.calls)
	}
}

func TestRouterPropagatesRetrievalError(t *testing.T) {
	t.Parallel()

	r, _, _ := newFixture(t, 0, errors.New("fts down"))
	if _, err := r.Route(context.Background(), "หุ่นยนต์พังแก้ยังไง"); err == nil {
		t.Fatal("Route() error = nil, want retrieval error")
	}
}

func TestNewEmbedsAnchorsAtBoot(t *testing.T) {
	t.Parallel()

	_, emb, _ := newFixture(t, 0, nil)

	seen := map[string]bool{}
	for _, batch := range emb.calls {
		for _, text := range batch {
			seen[text] = true
		}
	}
	for _, want := range []string{"ติดต่อเจ้าหน้าที่", "ตรวจสอบสถานะเคส", "เปิดเคสใหม่", "ใช้งานระบบนี้อย่างไร"} {
		if !seen[want] {
			t.Errorf("anchor %q was not embedded at boot", want)
		}
	}
}
