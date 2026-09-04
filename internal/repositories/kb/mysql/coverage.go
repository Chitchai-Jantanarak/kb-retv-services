package mysql

import (
	"strings"

	"github.com/my/app/internal/shared/coverage"
)

func companyFilter(column string, companyID int64, cov []int64) (string, []any) {
	ids := coverage.Normalize(cov, companyID)
	if len(ids) <= 1 {
		id := companyID
		if len(ids) == 1 {
			id = ids[0]
		}
		return column + " = ?", []any{id}
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	return column + " IN (" + strings.Join(placeholders, ",") + ")", args
}
