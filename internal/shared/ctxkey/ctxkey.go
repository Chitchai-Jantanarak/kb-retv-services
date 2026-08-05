package ctxkey

import "context"

type key int

const (
	companyID key = iota
	requestID
	loggerKey
	principalKey
	coverageKey
	citeCandidatesKey
)

type Principal struct {
	CompanyID int64
	UserID    int64
	Role      string
	Groups    []string
	Perms     []string
	Coverage  []int64
}

func WithCompanyID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, companyID, id)
}

func CompanyID(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(companyID).(int64)
	return v, ok
}

func MustCompanyID(ctx context.Context) int64 {
	v, ok := ctx.Value(companyID).(int64)
	if !ok {
		panic("company_id not found in context")
	}
	return v
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestID, id)
}

func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(requestID).(string)
	return v
}

func WithValue(ctx context.Context, v any) context.Context {
	return context.WithValue(ctx, loggerKey, v)
}

func Value(ctx context.Context) any {
	return ctx.Value(loggerKey)
}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	v, ok := ctx.Value(principalKey).(Principal)
	return v, ok
}

func MustPrincipal(ctx context.Context) Principal {
	v, ok := PrincipalFrom(ctx)
	if !ok {
		panic("principal not found in context")
	}
	return v
}

func WithCoverage(ctx context.Context, set []int64) context.Context {
	return context.WithValue(ctx, coverageKey, set)
}

func Coverage(ctx context.Context) ([]int64, bool) {
	v, ok := ctx.Value(coverageKey).([]int64)
	return v, ok
}

func CoverageOrSelf(ctx context.Context, companyID int64) []int64 {
	if v, ok := ctx.Value(coverageKey).([]int64); ok && len(v) > 0 {
		return v
	}
	return []int64{companyID}
}

func NormalizeCoverage(set []int64, companyID int64) []int64 {
	out := make([]int64, 0, len(set)+1)
	seen := make(map[int64]struct{}, len(set)+1)
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
	add(companyID)
	for _, id := range set {
		add(id)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func WithCiteCandidates(ctx context.Context, candidates []string) context.Context {
	if len(candidates) == 0 {
		return ctx
	}
	return context.WithValue(ctx, citeCandidatesKey, candidates)
}

func CiteCandidates(ctx context.Context) []string {
	candidates, _ := ctx.Value(citeCandidatesKey).([]string)
	return candidates
}
