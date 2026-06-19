package mysql

import "testing"

func TestCompanyFilter(t *testing.T) {
	cases := []struct {
		name       string
		companyID  int64
		coverage   []int64
		wantClause string
		wantArgs   []int64
	}{
		{name: "nil_coverage_equals", companyID: 3, coverage: nil, wantClause: "a.company_id = ?", wantArgs: []int64{3}},
		{name: "single_coverage_equals", companyID: 3, coverage: []int64{3}, wantClause: "a.company_id = ?", wantArgs: []int64{3}},
		{name: "subtree_uses_in", companyID: 3, coverage: []int64{3, 8}, wantClause: "a.company_id IN (?,?)", wantArgs: []int64{3, 8}},
		{name: "dedupes_and_drops_non_positive", companyID: 3, coverage: []int64{3, 3, 0, 8, -1}, wantClause: "a.company_id IN (?,?)", wantArgs: []int64{3, 8}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clause, args := companyFilter("a.company_id", tc.companyID, tc.coverage)
			if clause != tc.wantClause {
				t.Fatalf("clause = %q, want %q", clause, tc.wantClause)
			}
			if len(args) != len(tc.wantArgs) {
				t.Fatalf("args = %v, want %v", args, tc.wantArgs)
			}
			for i, want := range tc.wantArgs {
				if args[i] != want {
					t.Fatalf("args[%d] = %v, want %d", i, args[i], want)
				}
			}
		})
	}
}
