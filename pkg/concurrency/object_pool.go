package concurrency

import "sync"

type ObjectPool[T any] struct {
	pool sync.Pool
}

func NewObjectPool[T any](newValue func() T) *ObjectPool[T] {
	return &ObjectPool[T]{
		pool: sync.Pool{
			New: func() any {
				return newValue()
			},
		},
	}
}

func (p *ObjectPool[T]) Get() T {
	return p.pool.Get().(T)
}

func (p *ObjectPool[T]) Put(value T) {
	p.pool.Put(value)
}
