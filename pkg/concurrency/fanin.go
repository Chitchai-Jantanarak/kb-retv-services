package concurrency

import (
	"context"
	"sync"
)

func FanIn[T any](ctx context.Context, inputs ...<-chan T) <-chan T {
	out := make(chan T)
	var wg sync.WaitGroup
	wg.Add(len(inputs))

	for _, input := range inputs {
		go func(ch <-chan T) {
			defer wg.Done()
			for value := range ch {
				select {
				case <-ctx.Done():
					return
				case out <- value:
				}
			}
		}(input)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
