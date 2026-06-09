package ctxkey

import "context"

type key int

const (
	companyID key = iota
	requestID
	loggerKey
	principalKey
)

type Principal struct {
	CompanyID int64
	UserID    int64
	Role      string
	Groups    []string
	Perms     []string
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
