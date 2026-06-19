package ctxkey

import (
	"context"
	"testing"
)

func TestCompanyIDRoundTrip(t *testing.T) {
	ctx := WithCompanyID(context.Background(), 42)

	got, ok := CompanyID(ctx)
	if !ok {
		t.Fatal("CompanyID() ok = false, want true")
	}
	if got != 42 {
		t.Fatalf("CompanyID() = %d, want 42", got)
	}
	if MustCompanyID(ctx) != 42 {
		t.Fatalf("MustCompanyID() = %d, want 42", MustCompanyID(ctx))
	}
}

func TestCompanyIDMissing(t *testing.T) {
	if got, ok := CompanyID(context.Background()); ok || got != 0 {
		t.Fatalf("CompanyID() = %d, %v; want 0, false", got, ok)
	}
}

func TestMustCompanyIDPanicsWhenMissing(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustCompanyID() did not panic")
		}
	}()

	MustCompanyID(context.Background())
}

func TestCoverageRoundTrip(t *testing.T) {
	ctx := WithCoverage(context.Background(), []int64{1, 2, 3})

	got, ok := Coverage(ctx)
	if !ok || len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("Coverage() = %v, %v; want [1 2 3], true", got, ok)
	}
}

func TestCoverageOrSelfFallsBackToCompany(t *testing.T) {
	if got := CoverageOrSelf(context.Background(), 7); len(got) != 1 || got[0] != 7 {
		t.Fatalf("CoverageOrSelf(empty) = %v, want [7]", got)
	}

	ctx := WithCoverage(context.Background(), []int64{4, 5})
	if got := CoverageOrSelf(ctx, 7); len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Fatalf("CoverageOrSelf(set) = %v, want [4 5]", got)
	}
}

func TestNormalizeCoverage(t *testing.T) {
	cases := []struct {
		name      string
		set       []int64
		companyID int64
		want      []int64
	}{
		{name: "empty_falls_back_to_self", set: nil, companyID: 9, want: []int64{9}},
		{name: "company_first_then_set", set: []int64{2, 3}, companyID: 1, want: []int64{1, 2, 3}},
		{name: "dedupes_and_drops_non_positive", set: []int64{1, 0, 2, 2, -4}, companyID: 1, want: []int64{1, 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeCoverage(tc.set, tc.companyID)
			if len(got) != len(tc.want) {
				t.Fatalf("NormalizeCoverage = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("NormalizeCoverage = %v, want %v", got, tc.want)
				}
			}
		})
	}
}
