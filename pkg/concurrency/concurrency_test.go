package concurrency

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMapPreservesOrderAndLimitsConcurrency(t *testing.T) {
	var active int32
	var maxActive int32

	got, err := Map(context.Background(), []int{1, 2, 3, 4, 5}, 2, func(ctx context.Context, n int) (int, error) {
		current := atomic.AddInt32(&active, 1)
		for {
			seen := atomic.LoadInt32(&maxActive)
			if current <= seen || atomic.CompareAndSwapInt32(&maxActive, seen, current) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		return n * n, nil
	})
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1, 4, 9, 16, 25}) {
		t.Fatalf("unexpected results: %#v", got)
	}
	if maxActive > 2 {
		t.Fatalf("expected at most 2 active workers, got %d", maxActive)
	}
}

func TestMapReturnsFirstError(t *testing.T) {
	want := errors.New("boom")

	_, err := Map(context.Background(), []int{1, 2, 3}, 3, func(ctx context.Context, n int) (int, error) {
		if n == 2 {
			return 0, want
		}
		return n, nil
	})

	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestFanInMergesAndCloses(t *testing.T) {
	a := make(chan int, 2)
	b := make(chan int, 2)
	a <- 1
	a <- 2
	b <- 3
	close(a)
	close(b)

	var got []int
	for n := range FanIn(context.Background(), a, b) {
		got = append(got, n)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 values, got %#v", got)
	}
}

func TestPipelineComposesStages(t *testing.T) {
	source := SliceSource([]int{1, 2, 3})
	square := StageFunc[int, int](func(ctx context.Context, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for n := range in {
				out <- n * n
			}
		}()
		return out
	})
	toString := StageFunc[int, string](func(ctx context.Context, in <-chan int) <-chan string {
		out := make(chan string)
		go func() {
			defer close(out)
			for n := range in {
				out <- string(rune('0' + n))
			}
		}()
		return out
	})

	got := Collect(context.Background(), Pipe(context.Background(), Pipe(context.Background(), source, square), toString))
	if !reflect.DeepEqual(got, []string{"1", "4", "9"}) {
		t.Fatalf("unexpected pipeline output: %#v", got)
	}
}

func TestLimiterCapsConcurrentWork(t *testing.T) {
	limiter := NewLimiter(2)
	var active int32
	var maxActive int32
	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := limiter.Do(context.Background(), func() error {
				current := atomic.AddInt32(&active, 1)
				for {
					seen := atomic.LoadInt32(&maxActive)
					if current <= seen || atomic.CompareAndSwapInt32(&maxActive, seen, current) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				atomic.AddInt32(&active, -1)
				return nil
			}); err != nil {
				t.Errorf("limiter: %v", err)
			}
		}()
	}

	wg.Wait()
	if maxActive > 2 {
		t.Fatalf("expected at most 2 active calls, got %d", maxActive)
	}
}

func TestObjectPoolReturnsConfiguredValues(t *testing.T) {
	pool := NewObjectPool(func() *pooledValue {
		return &pooledValue{Name: "new"}
	})

	value := pool.Get()
	if value == nil {
		t.Fatal("Get() returned nil")
	}
	value.Name = "used"
	pool.Put(value)

	got := pool.Get()
	if got == nil {
		t.Fatal("Get() after Put() returned nil")
	}
	if got.Name != "used" && got.Name != "new" {
		t.Fatalf("Name = %q, want pooled or newly allocated value", got.Name)
	}
}

type pooledValue struct {
	Name string
}
