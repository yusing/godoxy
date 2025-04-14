package pool

import (
	"sort"

	"github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/utils/functional"
)

type (
	Pool[T Object] struct {
		m functional.Map[string, T]
	}
	Object interface {
		Key() string
		Name() string
		utils.MapMarshaller
	}
)

func New[T Object]() Pool[T] {
	return Pool[T]{functional.NewMapOf[string, T]()}
}

func (p Pool[T]) Add(obj T) {
	p.m.Store(obj.Key(), obj)
}

func (p Pool[T]) Del(obj T) {
	p.m.Delete(obj.Key())
}

func (p Pool[T]) Get(key string) (T, bool) {
	return p.m.Load(key)
}

func (p Pool[T]) Size() int {
	return p.m.Size()
}

func (p Pool[T]) Clear() {
	p.m.Clear()
}

func (p Pool[T]) Base() functional.Map[string, T] {
	return p.m
}

func (p Pool[T]) Slice() []T {
	slice := make([]T, 0, p.m.Size())
	for _, v := range p.m.Range {
		slice = append(slice, v)
	}
	sort.Slice(slice, func(i, j int) bool {
		return slice[i].Name() < slice[j].Name()
	})
	return slice
}

func (p Pool[T]) Iter(fn func(k string, v T) bool) {
	p.m.Range(fn)
}

func (p Pool[T]) IterAll(fn func(k string, v T)) {
	p.m.RangeAll(fn)
}
