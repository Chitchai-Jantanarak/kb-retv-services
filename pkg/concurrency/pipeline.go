package concurrency

import "context"

type Stage[I any, O any] interface {
	Run(context.Context, <-chan I) <-chan O
}

type StageFunc[I any, O any] func(context.Context, <-chan I) <-chan O

func (fn StageFunc[I, O]) Run(ctx context.Context, in <-chan I) <-chan O {
	return fn(ctx, in)
}

func Pipe[I any, O any](ctx context.Context, in <-chan I, stage Stage[I, O]) <-chan O {
	return stage.Run(ctx, in)
}

func SliceSource[T any](values []T) <-chan T {
	out := make(chan T)
	go func() {
		defer close(out)
		for _, value := range values {
			out <- value
		}
	}()
	return out
}

func Collect[T any](ctx context.Context, in <-chan T) []T {
	var values []T
	for {
		select {
		case <-ctx.Done():
			return values
		case value, ok := <-in:
			if !ok {
				return values
			}
			values = append(values, value)
		}
	}
}
