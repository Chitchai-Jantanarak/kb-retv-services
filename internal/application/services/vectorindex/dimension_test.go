package vectorindex

import (
	"context"
	"errors"
	"testing"

	"github.com/my/app/internal/application/services/kbbootstrap"
	"github.com/my/app/internal/domain/ports"
	"github.com/my/app/internal/shared/ctxkey"
)

func TestDecideDimensionAction(t *testing.T) {
	cases := []struct {
		name       string
		currentDim int
		exists     bool
		embedDim   int
		want       DimensionAction
		wantErr    bool
	}{
		{name: "absent collection is created", currentDim: 0, exists: false, embedDim: 768, want: DimensionCreated},
		{name: "live 3072 vs embedder 768 recreates", currentDim: 3072, exists: true, embedDim: 768, want: DimensionRecreated},
		{name: "live 768 vs embedder 3072 recreates", currentDim: 768, exists: true, embedDim: 3072, want: DimensionRecreated},
		{name: "matching size is unchanged", currentDim: 3072, exists: true, embedDim: 3072, want: DimensionUnchanged},
		{name: "absent ignores stale current dim", currentDim: 3072, exists: false, embedDim: 768, want: DimensionCreated},
		{name: "non-positive embedder dim errors", currentDim: 768, exists: true, embedDim: 0, wantErr: true},
		{name: "negative embedder dim errors", currentDim: 768, exists: false, embedDim: -1, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decideDimensionAction(tc.currentDim, tc.exists, tc.embedDim)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("decideDimensionAction(%d,%v,%d) err = nil, want error", tc.currentDim, tc.exists, tc.embedDim)
				}
				return
			}
			if err != nil {
				t.Fatalf("decideDimensionAction(%d,%v,%d) err = %v", tc.currentDim, tc.exists, tc.embedDim, err)
			}
			if got != tc.want {
				t.Fatalf("decideDimensionAction(%d,%v,%d) = %s, want %s", tc.currentDim, tc.exists, tc.embedDim, got, tc.want)
			}
		})
	}
}

func TestReconcileDimensionDrivesStore(t *testing.T) {
	cases := []struct {
		name         string
		currentDim   int
		exists       bool
		embedDim     int
		wantAction   DimensionAction
		wantEnsure   int
		wantRecreate int
	}{
		{name: "create when absent", exists: false, embedDim: 768, wantAction: DimensionCreated, wantEnsure: 768},
		{name: "recreate on mismatch", currentDim: 3072, exists: true, embedDim: 768, wantAction: DimensionRecreated, wantRecreate: 768},
		{name: "noop on match", currentDim: 768, exists: true, embedDim: 768, wantAction: DimensionUnchanged},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeReembedStore{currentDim: tc.currentDim, exists: tc.exists}
			action, err := ReconcileDimension(context.Background(), store, "kb_chunks__3", tc.embedDim)
			if err != nil {
				t.Fatalf("ReconcileDimension err = %v", err)
			}
			if action != tc.wantAction {
				t.Fatalf("action = %s, want %s", action, tc.wantAction)
			}
			if store.ensuredDim != tc.wantEnsure {
				t.Fatalf("ensured dim = %d, want %d", store.ensuredDim, tc.wantEnsure)
			}
			if store.recreatedDim != tc.wantRecreate {
				t.Fatalf("recreated dim = %d, want %d", store.recreatedDim, tc.wantRecreate)
			}
		})
	}
}

func TestReconcileDimensionPropagatesInspectError(t *testing.T) {
	store := &fakeReembedStore{inspectErr: errors.New("boom")}
	if _, err := ReconcileDimension(context.Background(), store, "kb_chunks__3", 768); err == nil {
		t.Fatal("ReconcileDimension err = nil, want inspect error")
	}
}

func TestReembedderRecreatesOnMismatchAndScopesByCompany(t *testing.T) {
	reports := &fakeReportReader{
		records: []kbbootstrap.ReportRecord{
			{ID: 5, CompanyID: 3, Code: "REP-5", Title: "printer offline", ProblemDetail: "queue stuck"},
		},
	}
	embed := stubEmbedder{dim: 768}
	store := &fakeReembedStore{currentDim: 3072, exists: true}
	reembedder := NewReembedder(reports, embed, store)

	result, err := reembedder.Run(context.Background(), ReembedOptions{
		CompanyID:        3,
		CollectionPrefix: "kb_chunks",
		Model:            "gemini-embedding-2",
	})
	if err != nil {
		t.Fatalf("Run() err = %v", err)
	}
	if result.Collection != "kb_chunks__3" {
		t.Fatalf("collection = %q, want kb_chunks__3", result.Collection)
	}
	if result.DimensionAction != DimensionRecreated {
		t.Fatalf("action = %s, want recreated", result.DimensionAction)
	}
	if result.Dim != 768 {
		t.Fatalf("dim = %d, want 768", result.Dim)
	}
	if store.recreatedDim != 768 {
		t.Fatalf("recreated dim = %d, want 768", store.recreatedDim)
	}
	if result.Reports != 1 || result.Points == 0 {
		t.Fatalf("reports=%d points=%d, want reports=1 points>0", result.Reports, result.Points)
	}
	if store.upsertCollection != "kb_chunks__3" {
		t.Fatalf("upsert collection = %q, want kb_chunks__3", store.upsertCollection)
	}
	for _, point := range store.points {
		if point.Metadata["company_id"] != int64(3) {
			t.Fatalf("point company_id = %v, want 3", point.Metadata["company_id"])
		}
		if len(point.Vector) != 768 {
			t.Fatalf("vector len = %d, want 768", len(point.Vector))
		}
	}
	if reports.sawCompanyID != 3 {
		t.Fatalf("reader company id = %d, want 3", reports.sawCompanyID)
	}
}

func TestReembedderRejectsCrossTenantReport(t *testing.T) {
	reports := &fakeReportReader{
		records: []kbbootstrap.ReportRecord{
			{ID: 9, CompanyID: 4, Title: "leaked"},
		},
	}
	store := &fakeReembedStore{exists: false}
	reembedder := NewReembedder(reports, stubEmbedder{dim: 768}, store)

	_, err := reembedder.Run(context.Background(), ReembedOptions{
		CompanyID:        3,
		CollectionPrefix: "kb_chunks",
	})
	if err == nil {
		t.Fatal("Run() err = nil, want cross-tenant rejection")
	}
	if len(store.points) != 0 {
		t.Fatalf("points upserted = %d, want 0", len(store.points))
	}
}

type fakeReembedStore struct {
	currentDim       int
	exists           bool
	inspectErr       error
	ensuredDim       int
	recreatedDim     int
	upsertCollection string
	points           []ports.VectorPoint
}

func (s *fakeReembedStore) CollectionVectorSize(ctx context.Context, name string) (int, bool, error) {
	if s.inspectErr != nil {
		return 0, false, s.inspectErr
	}
	return s.currentDim, s.exists, nil
}

func (s *fakeReembedStore) EnsureCollection(ctx context.Context, name string, dim int) error {
	s.ensuredDim = dim
	s.exists = true
	s.currentDim = dim
	return nil
}

func (s *fakeReembedStore) RecreateCollection(ctx context.Context, name string, dim int) error {
	s.recreatedDim = dim
	s.exists = true
	s.currentDim = dim
	return nil
}

func (s *fakeReembedStore) Upsert(ctx context.Context, collection string, points []ports.VectorPoint) error {
	s.upsertCollection = collection
	s.points = append(s.points, points...)
	return nil
}

type fakeReportReader struct {
	records      []kbbootstrap.ReportRecord
	sawCompanyID int64
}

func (r *fakeReportReader) Reports(ctx context.Context, companyID int64, limit int) ([]kbbootstrap.ReportRecord, error) {
	if _, ok := ctxkey.CompanyID(ctx); ok {
		r.sawCompanyID = companyID
	}
	return r.records, nil
}

type stubEmbedder struct {
	dim int
}

func (e stubEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(texts))
	for range texts {
		vec := make([]float32, e.dim)
		for i := range vec {
			vec[i] = float32(i)
		}
		vectors = append(vectors, vec)
	}
	return vectors, nil
}

func (e stubEmbedder) Dim() int { return e.dim }
