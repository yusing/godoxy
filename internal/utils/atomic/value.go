package atomic

import (
	"sync/atomic"

	"github.com/yusing/go-proxy/pkg/json"
)

type Value[T any] struct {
	atomic.Value
}

func (a *Value[T]) Load() T {
	if v := a.Value.Load(); v != nil {
		return v.(T)
	}
	var zero T
	return zero
}

func (a *Value[T]) Store(v T) {
	a.Value.Store(v)
}

func (a *Value[T]) Swap(v T) T {
	if v := a.Value.Swap(v); v != nil {
		return v.(T)
	}
	var zero T
	return zero
}

func (a *Value[T]) MarshalJSONTo(buf []byte) []byte {
	return json.MarshalTo(a.Load(), buf)
}
