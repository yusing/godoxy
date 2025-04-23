package jsonstore

import (
	"github.com/puzpuzpuz/xsync/v3"
)

type JSONStore[VT any] struct{ m jsonStoreInternal }

func NewStore[VT any](namespace namespace) JSONStore[VT] {
	storesMu.Lock()
	defer storesMu.Unlock()
	if s, ok := stores[namespace]; ok {
		return JSONStore[VT]{s}
	}
	m := jsonStoreInternal{xsync.NewMapOf[string, any]()}
	stores[namespace] = m
	return JSONStore[VT]{m}
}

func (s JSONStore[VT]) Load(key string) (_ VT, _ bool) {
	value, ok := s.m.Load(key)
	if !ok {
		return
	}
	return value.(VT), true
}

func (s JSONStore[VT]) Has(key string) bool {
	_, ok := s.m.Load(key)
	return ok
}

func (s JSONStore[VT]) Store(key string, value VT) {
	s.m.Store(key, value)
}

func (s JSONStore[VT]) Delete(key string) {
	s.m.Delete(key)
}

func (s JSONStore[VT]) Iter(yield func(key string, value VT) bool) {
	for k, v := range s.m.Range {
		if !yield(k, v.(VT)) {
			return
		}
	}
}
