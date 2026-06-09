package logger

import (
	"context"

	"go.uber.org/zap"

	"github.com/my/app/internal/shared/ctxkey"
)

func WithContext(ctx context.Context, l *zap.Logger) context.Context {
	return ctxkey.WithValue(ctx, l)
}

func FromContext(ctx context.Context) *zap.Logger {
	if l, ok := ctxkey.Value(ctx).(*zap.Logger); ok && l != nil {
		return l
	}
	return Get()
}
