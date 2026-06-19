package mysql

import "strings"

func companyFilter(column string, companyID int64, coverage []int64) (string, []any) {
	ids := normalizeCompanyIDs(coverage, companyID)
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

func normalizeCompanyIDs(coverage []int64, companyID int64) []int64 {
	out := make([]int64, 0, len(coverage)+1)
	seen := make(map[int64]struct{}, len(coverage)+1)
	add := func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range coverage {
		add(id)
	}
	if len(out) == 0 {
		add(companyID)
	}
	return out
}
