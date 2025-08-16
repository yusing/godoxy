package functional

import (
	"github.com/puzpuzpuz/xsync/v4"
)

type Set[T comparable] struct {
	m *xsync.Map[T, struct{}]
}

func NewSet[T comparable]() Set[T] {
	return Set[T]{m: xsync.NewMap[T, struct{}]()}
}

func (set Set[T]) Add(v T) {
	set.m.Store(v, struct{}{})
}

func (set Set[T]) Remove(v T) {
	set.m.Delete(v)
}

func (set Set[T]) Clear() {
	set.m.Clear()
}

func (set Set[T]) Contains(v T) bool {
	_, ok := set.m.Load(v)
	return ok
}

func (set Set[T]) Range(f func(T) bool) {
	set.m.Range(func(k T, _ struct{}) bool {
		return f(k)
	})
}

func (set Set[T]) RangeAll(f func(T)) {
	set.m.Range(func(k T, _ struct{}) bool {
		f(k)
		return true
	})
}

func (set Set[T]) Size() int {
	return set.m.Size()
}

func (set Set[T]) IsEmpty() bool {
	return set.m == nil || set.m.Size() == 0
}
