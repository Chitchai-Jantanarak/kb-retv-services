package graph

func coverageOrSelf(coverage []int64, companyID int64) []int64 {
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
