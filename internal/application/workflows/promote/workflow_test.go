package promote

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubRepo struct {
	getItem          ReviewItem
	getErr           error
	approvedRef      string
	approvedID       int64
	approvedErr      error
	activatedSymptom struct{ companyID, id int64 }
	activatedSubject struct{ companyID, id int64 }
	activateErr      error
}

func (r *stubRepo) Get(_ context.Context, _ int64) (ReviewItem, error) {
	return r.getItem, r.getErr
}

func (r *stubRepo) MarkApproved(_ context.Context, id int64, ref string) error {
	r.approvedID = id
	r.approvedRef = ref
	return r.approvedErr
}

func (r *stubRepo) ActivateSymptom(_ context.Context, companyID, id int64) error {
	if r.activateErr != nil {
		return r.activateErr
	}
	r.activatedSymptom = struct{ companyID, id int64 }{companyID, id}
	return nil
}

func (r *stubRepo) ActivateSubject(_ context.Context, companyID, id int64) error {
	if r.activateErr != nil {
		return r.activateErr
	}
	r.activatedSubject = struct{ companyID, id int64 }{companyID, id}
	return nil
}

func TestNewRejectsNilRepo(t *testing.T) {
	_, err := New(nil)
	if err == nil || !strings.Contains(err.Error(), "repo is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunFailureCases(t *testing.T) {
	cases := []struct {
		name    string
		repo    *stubRepo
		id      int64
		wantSub string
	}{
		{name: "non_positive_id", repo: &stubRepo{}, id: 0, wantSub: "review item id must be positive"},
		{name: "load_error", repo: &stubRepo{getErr: errors.New("db down")}, id: 1, wantSub: "load item"},
		{name: "no_company", repo: &stubRepo{getItem: ReviewItem{ID: 1, Kind: KindSymptom}}, id: 1, wantSub: "no company_id"},
		{name: "unsupported_kind", repo: &stubRepo{getItem: ReviewItem{ID: 1, CompanyID: 7, Kind: "weird"}}, id: 1, wantSub: "unsupported kind"},
		{name: "missing_payload_key", repo: &stubRepo{getItem: ReviewItem{ID: 1, CompanyID: 7, Kind: KindSymptom, Payload: map[string]any{}}}, id: 1, wantSub: "payload missing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wf, err := New(tc.repo)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			_, err = wf.Run(context.Background(), tc.id)
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("err = %v, want sub %q", err, tc.wantSub)
			}
		})
	}
}

func TestRunPromotesSymptom(t *testing.T) {
	repo := &stubRepo{getItem: ReviewItem{
		ID:        9,
		CompanyID: 7,
		Kind:      KindSymptom,
		Payload:   map[string]any{"symptom_node_id": float64(42)},
	}}
	wf, _ := New(repo)
	res, err := wf.Run(context.Background(), 9)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.PromotionRef != "symptom:42" {
		t.Fatalf("ref = %q", res.PromotionRef)
	}
	if repo.activatedSymptom.companyID != 7 || repo.activatedSymptom.id != 42 {
		t.Fatalf("symptom activate: %+v", repo.activatedSymptom)
	}
	if repo.approvedID != 9 || repo.approvedRef != "symptom:42" {
		t.Fatalf("approved: id=%d ref=%q", repo.approvedID, repo.approvedRef)
	}
}

func TestRunPromotesSubject(t *testing.T) {
	repo := &stubRepo{getItem: ReviewItem{
		ID:        10,
		CompanyID: 7,
		Kind:      KindSubject,
		Payload:   map[string]any{"subject_node_id": float64(99)},
	}}
	wf, _ := New(repo)
	res, err := wf.Run(context.Background(), 10)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.PromotionRef != "subject:99" {
		t.Fatalf("ref = %q", res.PromotionRef)
	}
	if repo.activatedSubject.companyID != 7 || repo.activatedSubject.id != 99 {
		t.Fatalf("subject activate: %+v", repo.activatedSubject)
	}
}

func TestRunPromotesKBSkeleton(t *testing.T) {
	repo := &stubRepo{getItem: ReviewItem{
		ID:        11,
		CompanyID: 7,
		Kind:      KindKB,
		Payload:   map[string]any{"kb_article_id": float64(200)},
	}}
	wf, _ := New(repo)
	res, err := wf.Run(context.Background(), 11)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.PromotionRef != "kb_article:200" {
		t.Fatalf("ref = %q", res.PromotionRef)
	}
}

func TestRunBubblesActivateError(t *testing.T) {
	repo := &stubRepo{
		getItem:     ReviewItem{ID: 9, CompanyID: 7, Kind: KindSymptom, Payload: map[string]any{"symptom_node_id": float64(42)}},
		activateErr: errors.New("table missing"),
	}
	wf, _ := New(repo)
	_, err := wf.Run(context.Background(), 9)
	if err == nil || !strings.Contains(err.Error(), "table missing") {
		t.Fatalf("err = %v", err)
	}
}
