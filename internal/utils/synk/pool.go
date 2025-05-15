package synk

import (
	"sync"
)

type (
	// Pool is a wrapper of sync.Pool that limits the size of the object.
	Pool[T any] struct {
		pool *sync.Pool
	}
	BytesPool = Pool[byte]
)

const DefaultInitBytes = 32 * 1024

func NewPool[T any](initSize int) *Pool[T] {
	return &Pool[T]{
		pool: &sync.Pool{
			New: func() any {
				return make([]T, 0, initSize)
			},
		},
	}
}

var bytesPool = NewPool[byte](DefaultInitBytes)

func NewBytesPool() *BytesPool {
	return bytesPool
}

func (p *Pool[T]) Get() []T {
	return p.pool.Get().([]T)
}

func (p *Pool[T]) GetSized(size int) []T {
	b := p.Get()
	if cap(b) < size {
		p.Put(b)
		return make([]T, size)
	}
	return b[:size]
}

func (p *Pool[T]) Put(b []T) {
	p.pool.Put(b[:0]) //nolint:staticcheck
}
