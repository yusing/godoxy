package pool

import (
	"sort"

	"github.com/puzpuzpuz/xsync/v3"
	"github.com/yusing/go-proxy/internal/logging"
)

type (
	Pool[T Object] struct {
		m    *xsync.MapOf[string, T]
		name string
	}
	Object interface {
		Key() string
		Name() string
	}
)

func New[T Object](name string) Pool[T] {
	return Pool[T]{xsync.NewMapOf[string, T](), name}
}

func (p Pool[T]) Name() string {
	return p.name
}

func (p Pool[T]) Add(obj T) {
	p.checkExists(obj.Key())
	p.m.Store(obj.Key(), obj)
	logging.Info().Msgf("%s: added %s", p.name, obj.Name())
}

func (p Pool[T]) Del(obj T) {
	p.m.Delete(obj.Key())
	logging.Info().Msgf("%s: removed %s", p.name, obj.Name())
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

func (p Pool[T]) Iter(fn func(k string, v T) bool) {
	p.m.Range(fn)
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
