package synk

import "sync"

type (
	// Pool is a wrapper of sync.Pool that limits the size of the object.
	Pool[T any] struct {
		pool    sync.Pool
		maxSize int
	}
	BytesPool = Pool[byte]
)

const (
	DefaultInitBytes = 1024
	DefaultMaxBytes  = 1024 * 1024
)

func NewPool[T any](initSize int, maxSize int) *Pool[T] {
	return &Pool[T]{
		pool: sync.Pool{
			New: func() any {
				return make([]T, 0, initSize)
			},
		},
		maxSize: maxSize,
	}
}

func NewBytesPool(initSize int, maxSize int) *BytesPool {
	return NewPool[byte](initSize, maxSize)
}

func (p *Pool[T]) Get() []T {
	return p.pool.Get().([]T)
}

func (p *Pool[T]) Put(b []T) {
	if cap(b) <= p.maxSize {
		p.pool.Put(b[:0])
	}
}
