package pool

import (
	"sort"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
)

type (
	Pool[T Object] struct {
		m          *xsync.Map[string, T]
		name       string
		disableLog atomic.Bool
	}
	// Preferable allows an object to express deterministic replacement preference
	// when multiple objects with the same key are added to the pool.
	// If new.PreferOver(old) returns true, the new object replaces the old one.
	Preferable interface {
		PreferOver(other any) bool
	}
	Object interface {
		Key() string
		Name() string
	}
	ObjectWithDisplayName interface {
		Object
		DisplayName() string
	}
)

func New[T Object](name string) Pool[T] {
	return Pool[T]{m: xsync.NewMap[string, T](), name: name}
}

func (p *Pool[T]) ToggleLog(v bool) {
	p.disableLog.Store(v)
}

func (p *Pool[T]) Name() string {
	return p.name
}

func (p *Pool[T]) Add(obj T) {
	p.AddKey(obj.Key(), obj)
}

func (p *Pool[T]) AddKey(key string, obj T) {
	if cur, exists := p.m.Load(key); exists {
		if newPref, ok := any(obj).(Preferable); ok {
			if !newPref.PreferOver(cur) {
				// keep existing
				return
			}
		}
	}
	p.checkExists(key)
	p.m.Store(key, obj)
	p.logAction("added", obj)
}

func (p *Pool[T]) AddIfNotExists(obj T) (actual T, added bool) {
	actual, loaded := p.m.LoadOrStore(obj.Key(), obj)
	if !loaded {
		p.logAction("added", obj)
	}
	return actual, !loaded
}

func (p *Pool[T]) Del(obj T) {
	p.m.Delete(obj.Key())
	p.logAction("removed", obj)
}

func (p *Pool[T]) DelKey(key string) {
	if v, exists := p.m.LoadAndDelete(key); exists {
		p.logAction("removed", v)
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

func (p *Pool[T]) logAction(action string, obj T) {
	if p.disableLog.Load() {
		return
	}
	if withName, ok := any(obj).(ObjectWithDisplayName); ok {
		disp, name := withName.DisplayName(), withName.Name()
		if disp != name {
			log.Info().Msgf("%s: %s %s (%s)", p.name, action, disp, name)
		} else {
			log.Info().Msgf("%s: %s %s", p.name, action, name)
		}
	} else {
		log.Info().Msgf("%s: %s %s", p.name, action, obj.Name())
	}
}
