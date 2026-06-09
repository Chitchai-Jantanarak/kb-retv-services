package concurrency

import (
	"context"
	"sync"
)

type MapFunc[I any, O any] func(context.Context, I) (O, error)

func Map[I any, O any](ctx context.Context, inputs []I, workers int, fn MapFunc[I, O]) ([]O, error) {
	if workers <= 0 {
		workers = 1
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type job struct {
		index int
		value I
	}

	jobs := make(chan job)
	results := make([]O, len(inputs))

	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	setErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		errMu.Unlock()
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case j, ok := <-jobs:
					if !ok {
						return
					}
					value, err := fn(ctx, j.value)
					if err != nil {
						setErr(err)
						return
					}
					results[j.index] = value
				}
			}
		}()
	}

sendLoop:
	for i, input := range inputs {
		select {
		case <-ctx.Done():
			break sendLoop
		case jobs <- job{index: i, value: input}:
		}
	}
	close(jobs)
	wg.Wait()

	errMu.Lock()
	defer errMu.Unlock()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil && err != context.Canceled {
		return nil, err
	}
	return results, nil
}
