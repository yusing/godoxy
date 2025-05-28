package task

import (
	"sync"

	"github.com/puzpuzpuz/xsync/v4"
)

type withWg[T comparable] struct {
	m  *xsync.Map[T, struct{}]
	wg sync.WaitGroup
}

func newWithWg[T comparable]() *withWg[T] {
	return &withWg[T]{
		m: xsync.NewMap[T, struct{}](),
	}
}

func (w *withWg[T]) Add(ele T) {
	w.wg.Add(1)
	w.m.Store(ele, struct{}{})
}

func (w *withWg[T]) AddWithoutWG(ele T) {
	w.m.Store(ele, struct{}{})
}

func (w *withWg[T]) Delete(key T) {
	w.wg.Done()
	w.m.Delete(key)
}

func (w *withWg[T]) DeleteWithoutWG(key T) {
	w.m.Delete(key)
}

func (w *withWg[T]) Wait() {
	w.wg.Wait()
}

func (w *withWg[T]) Range(yield func(T) bool) {
	for ele := range w.m.Range {
		if !yield(ele) {
			break
		}
	}
}
