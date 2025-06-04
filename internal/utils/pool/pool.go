package pool

import (
	"sort"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
)

type (
	Pool[T Object] struct {
		m          *xsync.Map[string, T]
		name       string
		disableLog bool
	}
	Object interface {
		Key() string
		Name() string
	}
)

func New[T Object](name string) Pool[T] {
	return Pool[T]{xsync.NewMap[string, T](), name, false}
}

func (p *Pool[T]) DisableLog() {
	p.disableLog = true
}

func (p *Pool[T]) Name() string {
	return p.name
}

func (p *Pool[T]) Add(obj T) {
	p.checkExists(obj.Key())
	p.m.Store(obj.Key(), obj)
	if !p.disableLog {
		log.Info().Msgf("%s: added %s", p.name, obj.Name())
	}
}

func (p *Pool[T]) AddKey(key string, obj T) {
	p.checkExists(key)
	p.m.Store(key, obj)
	if !p.disableLog {
		log.Info().Msgf("%s: added %s", p.name, obj.Name())
	}
}

func (p *Pool[T]) AddIfNotExists(obj T) (actual T, added bool) {
	actual, loaded := p.m.LoadOrStore(obj.Key(), obj)
	return actual, !loaded
}

func (p *Pool[T]) Del(obj T) {
	p.m.Delete(obj.Key())
	if !p.disableLog {
		log.Info().Msgf("%s: removed %s", p.name, obj.Name())
	}
}

func (p *Pool[T]) DelKey(key string) {
	p.m.Delete(key)
	if !p.disableLog {
		log.Info().Msgf("%s: removed %s", p.name, key)
	}
}

func (p *Pool[T]) Get(key string) (T, bool) {
	return p.m.Load(key)
}

func (p *Pool[T]) Size() int {
	return p.m.Size()
}

func (p *Pool[T]) Clear() {
	p.m.Clear()
}

func (p *Pool[T]) Iter(fn func(k string, v T) bool) {
	p.m.Range(fn)
}

func (p *Pool[T]) Slice() []T {
	slice := make([]T, 0, p.m.Size())
	for _, v := range p.m.Range {
		slice = append(slice, v)
	}
	sort.Slice(slice, func(i, j int) bool {
		return slice[i].Name() < slice[j].Name()
	})
	return slice
}
