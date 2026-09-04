package graph

import cov "github.com/my/app/internal/shared/coverage"

func coverageOrSelf(coverage []int64, companyID int64) []int64 {
	return cov.Normalize(coverage, companyID)
}
