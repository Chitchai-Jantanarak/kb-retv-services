package rag

import (
	"context"

	"github.com/my/app/internal/shared/ctxkey"
)

func queryCoverage(ctx context.Context, query Query, companyID int64) []int64 {
	if len(query.Coverage) > 0 {
		return query.Coverage
	}
	return ctxkey.CoverageOrSelf(ctx, companyID)
}

func ftsCompanyIDs(coverage []int64) []int64 {
	out := make([]int64, 0, len(coverage))
	seen := make(map[int64]struct{}, len(coverage))
	for _, id := range coverage {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
